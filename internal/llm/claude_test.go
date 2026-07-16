package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/message"
)

// TestClaudeClientSendsMessagesHeadersAndDecodesResponse 는 Claude adapter가 내부 요청을 Messages API 형태로 변환하고 응답 tool_use를 보존하는지 확인한다.
func TestClaudeClientSendsMessagesHeadersAndDecodesResponse(t *testing.T) {
	const apiKey = "secret-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Method = %s, want POST", r.Method)
		}
		if r.URL.Path != claudeMessagesPath {
			t.Fatalf("Path = %s, want %s", r.URL.Path, claudeMessagesPath)
		}
		if got := r.Header.Get("x-api-key"); got != apiKey {
			t.Fatalf("x-api-key = %q, want api key", got)
		}
		if got := r.Header.Get("anthropic-version"); got != claudeAPIVersion {
			t.Fatalf("anthropic-version = %q, want %q", got, claudeAPIVersion)
		}
		if got := r.Header.Get("Content-Type"); got != claudeRequestMediaType {
			t.Fatalf("Content-Type = %q, want %q", got, claudeRequestMediaType)
		}

		var req claudeMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		if req.Model != "claude-test" {
			t.Fatalf("Model = %q, want claude-test", req.Model)
		}
		if req.MaxTokens != claudeDefaultMaxTokens {
			t.Fatalf("MaxTokens = %d, want %d", req.MaxTokens, claudeDefaultMaxTokens)
		}
		if req.System != "runtime rule" {
			t.Fatalf("System = %q, want runtime rule", req.System)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("len(Messages) = %d, want 2", len(req.Messages))
		}
		if len(req.Tools) != 1 {
			t.Fatalf("len(Tools) = %d, want 1", len(req.Tools))
		}
		if req.Tools[0].Name != "search" || req.Tools[0].Description != "Search documents" {
			t.Fatalf("tool = %+v, want search schema", req.Tools[0])
		}
		if string(req.Tools[0].InputSchema) != `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}` {
			t.Fatalf("tool input_schema = %s, want query schema", req.Tools[0].InputSchema)
		}
		if req.Messages[0].Role != "user" || req.Messages[0].Content[0].Text != "hello" {
			t.Fatalf("first message = %+v, want user text", req.Messages[0])
		}
		if req.Messages[1].Role != "assistant" {
			t.Fatalf("second role = %q, want assistant", req.Messages[1].Role)
		}
		if len(req.Messages[1].Content) != 2 {
			t.Fatalf("assistant content len = %d, want text and tool_use", len(req.Messages[1].Content))
		}
		if req.Messages[1].Content[1].Type != "tool_use" || req.Messages[1].Content[1].Name != "search" {
			t.Fatalf("assistant tool_use block = %+v, want search tool_use", req.Messages[1].Content[1])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"claude-test",
			"role":"assistant",
			"stop_reason":"tool_use",
			"usage":{"input_tokens":7,"output_tokens":11},
			"content":[
				{"type":"text","text":"checking"},
				{"type":"tool_use","id":"toolu_1","name":"search","input":{"query":"agent runtime"}}
			]
		}`))
	}))
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{
		Model:  "claude-test",
		APIKey: apiKey,
	}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Tools: []message.ToolSchema{
			{
				Name:        "search",
				Description: "Search documents",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
			},
		},
		Messages: []message.Message{
			message.System("runtime rule"),
			message.User("hello"),
			message.Assistant("previous", message.ToolCall{
				ID:        "toolu_previous",
				Name:      "search",
				Arguments: json.RawMessage(`{"query":"before"}`),
			}),
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if resp.Provider != ProviderClaude || resp.Model != "claude-test" {
		t.Fatalf("response provider/model = %q/%q, want claude/claude-test", resp.Provider, resp.Model)
	}
	if resp.Message.Role != message.RoleAssistant || resp.Message.Text != "checking" {
		t.Fatalf("response message = %+v, want assistant text", resp.Message)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.ID != "toolu_1" || call.Name != "search" || string(call.Arguments) != `{"query":"agent runtime"}` {
		t.Fatalf("ToolCall = %+v, want toolu_1 search args", call)
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("StopReason = %q, want tool_use", resp.StopReason)
	}
	if resp.FinishReason != FinishReasonToolCall {
		t.Fatalf("FinishReason = %q, want %q", resp.FinishReason, FinishReasonToolCall)
	}
	if resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 11 || resp.Usage.TotalTokens != 18 {
		t.Fatalf("Usage = %+v, want 7/11/18", resp.Usage)
	}
}

func TestClaudeFinishReasonNormalization(t *testing.T) {
	tests := []struct {
		stopReason string
		want       FinishReason
	}{
		{stopReason: "end_turn", want: FinishReasonComplete},
		{stopReason: "stop_sequence", want: FinishReasonComplete},
		{stopReason: "tool_use", want: FinishReasonToolCall},
		{stopReason: "max_tokens", want: FinishReasonLengthLimit},
		{stopReason: "refusal", want: FinishReasonBlocked},
		{stopReason: "pause_turn", want: FinishReasonUnknown},
		{stopReason: "future_reason", want: FinishReasonUnknown},
		{want: FinishReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.stopReason, func(t *testing.T) {
			if got := normalizeClaudeFinishReason(tt.stopReason); got != tt.want {
				t.Fatalf("normalizeClaudeFinishReason(%q) = %q, want %q", tt.stopReason, got, tt.want)
			}
		})
	}
}

// TestNewClaudeClientRejectsMissingRequiredConfig 는 Claude 호출 전에 model과 API key 누락을 설정 오류로 거절하는지 확인한다.
func TestNewClaudeClientRejectsMissingRequiredConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  ProviderConfig
	}{
		{name: "model", cfg: ProviderConfig{APIKey: "secret"}},
		{name: "api key", cfg: ProviderConfig{Model: "claude-test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClaudeClient(tt.cfg)
			if err == nil {
				t.Fatal("NewClaudeClient() error = nil, want config error")
			}
			if !IsKind(err, ErrorKindConfig) {
				t.Fatalf("NewClaudeClient() error kind mismatch: %v", err)
			}
		})
	}
}

// TestClaudeClientHTTPErrorDoesNotExposeAPIKey 는 provider 오류 메시지에 API key가 섞여 들어가도 외부 오류에서는 마스킹되는지 확인한다.
func TestClaudeClientHTTPErrorDoesNotExposeAPIKey(t *testing.T) {
	const apiKey = "secret-key"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad key secret-key"}}`))
	}))
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{
		Model:  "claude-test",
		APIKey: apiKey,
	}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{Messages: []message.Message{message.User("hello")}})
	if err == nil {
		t.Fatal("Chat() error = nil, want provider error")
	}
	if !IsKind(err, ErrorKindProvider) {
		t.Fatalf("Chat() error kind mismatch: %v", err)
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatalf("Chat() error exposed API key: %v", err)
	}
	if !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("Chat() error = %v, want redacted marker", err)
	}
}

// TestClaudeClientTimeoutUsesTimeoutErrorKind 는 context deadline 초과가 일반 provider 오류가 아니라 timeout 오류로 분류되는지 확인한다.
func TestClaudeClientTimeoutUsesTimeoutErrorKind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	client, err := newClaudeClient(ProviderConfig{
		Model:  "claude-test",
		APIKey: "secret",
	}, server.Client(), server.URL)
	if err != nil {
		t.Fatalf("newClaudeClient() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	_, err = client.Chat(ctx, ChatRequest{Messages: []message.Message{message.User("hello")}})
	if err == nil {
		t.Fatal("Chat() error = nil, want timeout error")
	}
	if !IsKind(err, ErrorKindTimeout) {
		t.Fatalf("Chat() error kind mismatch: %v", err)
	}
}
