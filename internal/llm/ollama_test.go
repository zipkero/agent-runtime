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

// TestOllamaClientSendsChatRequestAndDecodesResponse 는 내부 요청을 Ollama Chat API 형태로 변환하고 응답 tool_calls를 보존하는지 확인한다.
func TestOllamaClientSendsChatRequestAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("Method = %s, want POST", r.Method)
		}
		if r.URL.Path != ollamaChatPath {
			t.Fatalf("Path = %s, want %s", r.URL.Path, ollamaChatPath)
		}
		if got := r.Header.Get("Content-Type"); got != ollamaRequestMediaType {
			t.Fatalf("Content-Type = %q, want %q", got, ollamaRequestMediaType)
		}

		var req ollamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode request body: %v", err)
		}
		if req.Model != "ollama-test" {
			t.Fatalf("Model = %q, want ollama-test", req.Model)
		}
		if req.Stream {
			t.Fatal("Stream = true, want false")
		}
		if len(req.Messages) != 4 {
			t.Fatalf("len(Messages) = %d, want 4", len(req.Messages))
		}
		if req.Messages[0].Role != "system" || req.Messages[0].Content != "runtime rule" {
			t.Fatalf("system message = %+v, want system text", req.Messages[0])
		}
		if req.Messages[1].Role != "user" || req.Messages[1].Content != "hello" {
			t.Fatalf("user message = %+v, want user text", req.Messages[1])
		}
		if req.Messages[2].Role != "assistant" || len(req.Messages[2].ToolCalls) != 1 {
			t.Fatalf("assistant message = %+v, want assistant tool call", req.Messages[2])
		}
		if req.Messages[2].ToolCalls[0].Function.Name != "search" ||
			string(req.Messages[2].ToolCalls[0].Function.Arguments) != `{"query":"before"}` {
			t.Fatalf("assistant tool call = %+v, want search args", req.Messages[2].ToolCalls[0])
		}
		if req.Messages[3].Role != "tool" || req.Messages[3].ToolResultName != "search" || req.Messages[3].Content != "result text" {
			t.Fatalf("tool message = %+v, want tool result", req.Messages[3])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"ollama-test",
			"message":{
				"role":"assistant",
				"content":"checking",
				"tool_calls":[
					{"function":{"name":"search","arguments":{"query":"agent runtime"}}}
				]
			},
			"done_reason":"stop",
			"done":true,
			"prompt_eval_count":5,
			"eval_count":13
		}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{
		Model: "ollama-test",
		Host:  server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			message.System("runtime rule"),
			message.User("hello"),
			message.Assistant("previous", message.ToolCall{
				ID:        "call_previous",
				Name:      "search",
				Arguments: json.RawMessage(`{"query":"before"}`),
			}),
			message.Tool(message.ToolResult{
				ToolCallID: "call_previous",
				Name:       "search",
				Content:    "result text",
			}),
		},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if resp.Provider != ProviderOllama || resp.Model != "ollama-test" {
		t.Fatalf("response provider/model = %q/%q, want ollama/ollama-test", resp.Provider, resp.Model)
	}
	if resp.Message.Role != message.RoleAssistant || resp.Message.Text != "checking" {
		t.Fatalf("response message = %+v, want assistant text", resp.Message)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.Message.ToolCalls))
	}
	call := resp.Message.ToolCalls[0]
	if call.Name != "search" || string(call.Arguments) != `{"query":"agent runtime"}` {
		t.Fatalf("ToolCall = %+v, want search args", call)
	}
	if resp.StopReason != "stop" {
		t.Fatalf("StopReason = %q, want stop", resp.StopReason)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 13 || resp.Usage.TotalTokens != 18 {
		t.Fatalf("Usage = %+v, want 5/13/18", resp.Usage)
	}
}

// TestNewOllamaClientRejectsMissingRequiredConfig 는 Ollama 호출 전에 model과 host 누락을 설정 오류로 거절하는지 확인한다.
func TestNewOllamaClientRejectsMissingRequiredConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  ProviderConfig
	}{
		{name: "model", cfg: ProviderConfig{Host: "http://localhost:11434"}},
		{name: "host", cfg: ProviderConfig{Model: "ollama-test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewOllamaClient(tt.cfg)
			if err == nil {
				t.Fatal("NewOllamaClient() error = nil, want config error")
			}
			if !IsKind(err, ErrorKindConfig) {
				t.Fatalf("NewOllamaClient() error kind mismatch: %v", err)
			}
		})
	}
}

// TestOllamaClientHTTPErrorUsesProviderError 는 Ollama 오류 응답을 provider 오류로 분류하고 메시지를 보존하는지 확인한다.
func TestOllamaClientHTTPErrorUsesProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{
		Model: "ollama-test",
		Host:  server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{Messages: []message.Message{message.User("hello")}})
	if err == nil {
		t.Fatal("Chat() error = nil, want provider error")
	}
	if !IsKind(err, ErrorKindProvider) {
		t.Fatalf("Chat() error kind mismatch: %v", err)
	}
	if !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("Chat() error = %v, want provider message", err)
	}
}

// TestOllamaClientTimeoutUsesTimeoutErrorKind 는 context deadline 초과가 일반 provider 오류가 아니라 timeout 오류로 분류되는지 확인한다.
func TestOllamaClientTimeoutUsesTimeoutErrorKind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	client, err := newOllamaClient(ProviderConfig{
		Model: "ollama-test",
		Host:  server.URL,
	}, server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient() error = %v", err)
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
