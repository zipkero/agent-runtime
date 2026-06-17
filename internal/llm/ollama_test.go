package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/message"
)

// TestNewOllamaClientRequiresHostAndModel 은 host 또는 model이 비면 생성자가 error를
// 반환하는지 검증한다.
func TestNewOllamaClientRequiresHostAndModel(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
	}{
		{
			name: "host 부재",
			cfg:  config.Config{Model: "llama3"},
		},
		{
			name: "model 부재",
			cfg:  config.Config{Host: "http://localhost:11434"},
		},
		{
			name: "host와 model 모두 부재",
			cfg:  config.Config{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewOllamaClient(tc.cfg)
			if err == nil {
				t.Fatalf("expected error when %s, got nil", tc.name)
			}
		})
	}
}

// TestOllamaClientMapsRequestAndResponse 는 httptest로 /api/chat을 가로채
// 요청 body에 system·user 메시지와 stream:false·model이 실리는지,
// 응답 text가 assistant 메시지로 환원되는지 검증한다.
func TestOllamaClientMapsRequestAndResponse(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message": {
				"role": "assistant",
				"content": "안녕하세요, 무엇을 도와드릴까요?"
			}
		}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleSystem,
				Content: []message.ContentBlock{message.NewTextBlock("항상 한국어로 답변하라.")},
			},
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("안녕하세요.")},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	assertOllamaRequest(t, requestBody)
	assertOllamaResponse(t, resp)
}

// TestOllamaClientUsesRequestModelOverride 는 req.Model이 있으면 client 기본 model 대신
// 해당 값이 요청 body에 실리는지 검증한다.
func TestOllamaClientUsesRequestModelOverride(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message": {"role": "assistant", "content": "ok"}}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "default-model", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{
		Model: "override-model",
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("hello")},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if requestBody["model"] != "override-model" {
		t.Fatalf("expected request model override, got %v", requestBody["model"])
	}
}

// TestOllamaClientReturnsContextErrorOnCancellation 은 취소된 ctx로 Chat을 호출하면
// context error가 그대로 표면화되는지 검증한다.
func TestOllamaClientReturnsContextErrorOnCancellation(t *testing.T) {
	client, err := newOllamaClient("http://localhost:11434", "llama3", http.DefaultClient)
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err = client.Chat(ctx, ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("hello")},
			},
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded error, got %v", err)
	}
}

// TestOllamaClientReturnsContextErrorOnCancellationDuringRequest 는 요청 중 ctx가 취소되면
// context error가 표면화되는지 검증한다.
func TestOllamaClientReturnsContextErrorOnCancellationDuringRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ctx가 취소될 때까지 기다려 in-flight 취소를 시뮬레이션한다.
		<-r.Context().Done()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 요청이 서버에 도달한 직후 취소되도록 별도 goroutine에서 cancel을 호출한다.
	go cancel()

	_, err = client.Chat(ctx, ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("hello")},
			},
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

// TestOllamaClientReturnsErrorOnNonOKStatus 는 서버가 비정상 status를 반환하면 error가
// 표면화되는지 검증한다.
func TestOllamaClientReturnsErrorOnNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("hello")},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error on non-OK status, got nil")
	}
}

// TestOllamaClientToolsSchemaMapping 은 req.Tools가 올바른 Ollama wire 형태로 변환되는지
// 검증한다. type/properties/required 분리 및 나머지 키 보존을 확인한다.
func TestOllamaClientToolsSchemaMapping(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message": {"role": "assistant", "content": "ok"}}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient: %v", err)
	}

	schema := json.RawMessage(`{
		"type": "object",
		"properties": {"query": {"type": "string"}},
		"required": ["query"],
		"additionalProperties": false
	}`)

	_, err = client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{Role: message.RoleUser, Content: []message.ContentBlock{message.NewTextBlock("search")}},
		},
		Tools: []message.ToolSpec{
			{Name: "web_search", Description: "웹 검색", InputSchema: schema},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	// tools[] 검증
	tools, ok := requestBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %#v", requestBody["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["type"] != "function" {
		t.Fatalf("unexpected tool: %#v", tools[0])
	}
	fn, ok := tool["function"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected function: %#v", tool["function"])
	}
	if fn["name"] != "web_search" {
		t.Fatalf("unexpected tool name: %v", fn["name"])
	}
	if fn["description"] != "웹 검색" {
		t.Fatalf("unexpected tool description: %v", fn["description"])
	}

	// parameters 내 type·properties·required·additionalProperties 보존 확인
	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters is not a map: %#v", fn["parameters"])
	}
	if params["type"] != "object" {
		t.Fatalf("expected type=object, got %v", params["type"])
	}
	if params["properties"] == nil {
		t.Fatalf("expected properties, got nil")
	}
	required, ok := params["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "query" {
		t.Fatalf("unexpected required: %#v", params["required"])
	}
	// 나머지 키(additionalProperties) 보존 확인
	if params["additionalProperties"] != false {
		t.Fatalf("expected additionalProperties=false, got %v", params["additionalProperties"])
	}
}

// TestOllamaClientAssistantToolCallsMapping 은 assistant 메시지의 tool_call 블록이
// wire tool_calls[]로 변환되는지 검증한다.
func TestOllamaClientAssistantToolCallsMapping(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message": {"role": "assistant", "content": "완료"}}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient: %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("검색해줘")},
			},
			{
				Role: message.RoleAssistant,
				Content: []message.ContentBlock{
					message.NewToolCallBlock(message.ToolCall{
						ID:    "call_0",
						Name:  "web_search",
						Input: json.RawMessage(`{"query":"Go testing"}`),
					}),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %#v", requestBody["messages"])
	}

	assistantMsg, ok := messages[1].(map[string]any)
	if !ok || assistantMsg["role"] != "assistant" {
		t.Fatalf("unexpected assistant message: %#v", messages[1])
	}

	toolCalls, ok := assistantMsg["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool_call, got %#v", assistantMsg["tool_calls"])
	}

	tc, ok := toolCalls[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected tool_call: %#v", toolCalls[0])
	}
	if tc["id"] != "call_0" {
		t.Fatalf("unexpected tool_call id: %v", tc["id"])
	}
	fn, ok := tc["function"].(map[string]any)
	if !ok || fn["name"] != "web_search" {
		t.Fatalf("unexpected tool_call function: %#v", tc["function"])
	}
}

// TestOllamaClientToolResultOneToN 은 RoleTool 메시지의 tool_result 블록이
// 각각 개별 role:"tool" wire 메시지로 1:N 변환되는지 검증한다.
func TestOllamaClientToolResultOneToN(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message": {"role": "assistant", "content": "완료"}}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient: %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("검색해줘")},
			},
			{
				Role: message.RoleAssistant,
				Content: []message.ContentBlock{
					message.NewToolCallBlock(message.ToolCall{
						ID:    "call_0",
						Name:  "web_search",
						Input: json.RawMessage(`{"query":"A"}`),
					}),
					message.NewToolCallBlock(message.ToolCall{
						ID:    "call_1",
						Name:  "calculator",
						Input: json.RawMessage(`{"expression":"1+1"}`),
					}),
				},
			},
			{
				// tool_result 2개를 가진 단일 RoleTool 메시지 → wire에서 2개 메시지로 분리
				Role: message.RoleTool,
				Content: []message.ContentBlock{
					message.NewToolResultBlock(message.ToolResult{
						ToolCallID: "call_0",
						Content:    "결과A",
					}),
					message.NewToolResultBlock(message.ToolResult{
						ToolCallID: "call_1",
						Content:    "결과B",
					}),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	messages, ok := requestBody["messages"].([]any)
	// user(1) + assistant tool_calls(1) + tool_result 2개 분리(2) = 4개
	if !ok || len(messages) != 4 {
		t.Fatalf("expected 4 messages after 1:N expansion, got %d: %#v", len(messages), requestBody["messages"])
	}

	for i, want := range []struct {
		id   string
		name string
	}{
		{id: "call_0", name: "web_search"},
		{id: "call_1", name: "calculator"},
	} {
		msg, ok := messages[i+2].(map[string]any)
		if !ok || msg["role"] != "tool" {
			t.Fatalf("expected role:tool at index %d, got %#v", i+2, messages[i+2])
		}
		if msg["tool_call_id"] != want.id {
			t.Fatalf("expected tool_call_id=%s at index %d, got %v", want.id, i+2, msg["tool_call_id"])
		}
		if msg["tool_name"] != want.name {
			t.Fatalf("expected tool_name=%s at index %d, got %v", want.name, i+2, msg["tool_name"])
		}
	}
}

// TestOllamaClientResponseToolCallConversion 은 응답 tool_calls[]가 internal tool_call
// 블록으로 환원되는지 검증하며, id가 있으면 그대로, 비어 있으면 결정적 ID(call_<n>)를 부여한다.
func TestOllamaClientResponseToolCallConversion(t *testing.T) {
	cases := []struct {
		name       string
		serverResp string
		wantIDs    []string
	}{
		{
			name: "id 있는 응답",
			serverResp: `{"message": {"role": "assistant", "tool_calls": [
				{"id": "abc123", "function": {"name": "web_search", "arguments": {"query": "Go"}}}
			]}}`,
			wantIDs: []string{"abc123"},
		},
		{
			name: "id 없는 응답(결정적 ID 부여)",
			serverResp: `{"message": {"role": "assistant", "tool_calls": [
				{"function": {"name": "web_search", "arguments": {"query": "Go"}}},
				{"function": {"name": "calc", "arguments": {"expr": "1+1"}}}
			]}}`,
			wantIDs: []string{"call_0", "call_1"},
		},
		{
			name: "text와 tool_call 혼합",
			serverResp: `{"message": {"role": "assistant", "content": "검색합니다", "tool_calls": [
				{"function": {"name": "web_search", "arguments": {"query": "Go"}}}
			]}}`,
			wantIDs: []string{"call_0"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.serverResp))
			}))
			defer server.Close()

			client, err := newOllamaClient(server.URL, "llama3", server.Client())
			if err != nil {
				t.Fatalf("newOllamaClient: %v", err)
			}

			resp, err := client.Chat(context.Background(), ChatRequest{
				Messages: []message.Message{
					{Role: message.RoleUser, Content: []message.ContentBlock{message.NewTextBlock("hello")}},
				},
			})
			if err != nil {
				t.Fatalf("Chat: %v", err)
			}

			// tool_call 블록 추출
			var toolCalls []message.ContentBlock
			for _, b := range resp.Message.Content {
				if b.Type == message.BlockTypeToolCall {
					toolCalls = append(toolCalls, b)
				}
			}

			if len(toolCalls) != len(tc.wantIDs) {
				t.Fatalf("expected %d tool_call blocks, got %d", len(tc.wantIDs), len(toolCalls))
			}

			for i, want := range tc.wantIDs {
				got := toolCalls[i].ToolCall.ID
				if got != want {
					t.Fatalf("tool_call[%d] ID: want %q, got %q", i, want, got)
				}
			}
		})
	}
}

// TestOllamaClientToolCallIDRoundtrip 은 응답 tool_call ID가 다음 요청의
// assistant tool_calls[].id와 tool 메시지 tool_call_id로 동일하게 왕복되는지 검증한다.
func TestOllamaClientToolCallIDRoundtrip(t *testing.T) {
	// 1차 요청: tool_call 응답(id 없음 → call_0 부여)
	// 2차 요청: call_0이 assistant tool_calls·tool 메시지에 실리는지 확인
	var secondRequestBody map[string]any

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++
		if callCount == 1 {
			// 1차: id 없는 tool_call 응답
			_, _ = w.Write([]byte(`{"message": {"role": "assistant", "tool_calls": [
				{"function": {"name": "web_search", "arguments": {"query": "Go"}}}
			]}}`))
		} else {
			// 2차 요청 body를 캡처한다.
			if err := json.NewDecoder(r.Body).Decode(&secondRequestBody); err != nil {
				t.Errorf("decode second request body: %v", err)
			}
			_, _ = w.Write([]byte(`{"message": {"role": "assistant", "content": "최종 응답"}}`))
		}
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient: %v", err)
	}

	// 1차 요청
	firstResp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{Role: message.RoleUser, Content: []message.ContentBlock{message.NewTextBlock("검색해줘")}},
		},
	})
	if err != nil {
		t.Fatalf("1차 Chat: %v", err)
	}

	// 응답에서 tool_call ID 확인
	if len(firstResp.Message.Content) == 0 || firstResp.Message.Content[0].Type != message.BlockTypeToolCall {
		t.Fatalf("expected tool_call block in first response")
	}
	toolCallID := firstResp.Message.Content[0].ToolCall.ID
	if toolCallID != "call_0" {
		t.Fatalf("expected call_0, got %s", toolCallID)
	}

	// 2차 요청: 1차 응답 + tool 결과를 history에 포함
	_, err = client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{Role: message.RoleUser, Content: []message.ContentBlock{message.NewTextBlock("검색해줘")}},
			firstResp.Message, // assistant(tool_call call_0)
			{
				Role: message.RoleTool,
				Content: []message.ContentBlock{
					message.NewToolResultBlock(message.ToolResult{
						ToolCallID: toolCallID, // call_0
						Content:    "검색 결과",
					}),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("2차 Chat: %v", err)
	}

	// 2차 요청의 wire messages 확인
	msgs, ok := secondRequestBody["messages"].([]any)
	// user + assistant(tool_calls) + tool(결과) = 3개
	if !ok || len(msgs) != 3 {
		t.Fatalf("expected 3 messages in second request, got %d", len(msgs))
	}

	// assistant 메시지의 tool_calls[].id = call_0
	assistantMsg, ok := msgs[1].(map[string]any)
	if !ok {
		t.Fatalf("unexpected assistant msg: %#v", msgs[1])
	}
	toolCalls, ok := assistantMsg["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool_call in assistant msg: %#v", assistantMsg["tool_calls"])
	}
	tc, ok := toolCalls[0].(map[string]any)
	if !ok || tc["id"] != "call_0" {
		t.Fatalf("expected call_0 in assistant tool_calls, got %#v", tc)
	}

	// tool 메시지의 tool_call_id = call_0
	toolMsg, ok := msgs[2].(map[string]any)
	if !ok || toolMsg["role"] != "tool" {
		t.Fatalf("unexpected tool msg: %#v", msgs[2])
	}
	if toolMsg["tool_call_id"] != "call_0" {
		t.Fatalf("expected tool_call_id=call_0, got %v", toolMsg["tool_call_id"])
	}
	if toolMsg["tool_name"] != "web_search" {
		t.Fatalf("expected tool_name=web_search, got %v", toolMsg["tool_name"])
	}
}

// assertOllamaRequest 는 가로챈 요청 body에 필수 필드가 올바르게 실렸는지 검증한다.
func assertOllamaRequest(t *testing.T, requestBody map[string]any) {
	t.Helper()

	// model 필드
	if requestBody["model"] != "llama3" {
		t.Fatalf("unexpected model: %v", requestBody["model"])
	}

	// stream:false
	if requestBody["stream"] != false {
		t.Fatalf("expected stream:false, got %v", requestBody["stream"])
	}

	// messages: system + user 두 개
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected messages: %#v", requestBody["messages"])
	}

	systemMsg, ok := messages[0].(map[string]any)
	if !ok || systemMsg["role"] != "system" {
		t.Fatalf("unexpected system message: %#v", messages[0])
	}
	if systemMsg["content"] != "항상 한국어로 답변하라." {
		t.Fatalf("unexpected system message content: %v", systemMsg["content"])
	}

	userMsg, ok := messages[1].(map[string]any)
	if !ok || userMsg["role"] != "user" {
		t.Fatalf("unexpected user message: %#v", messages[1])
	}
	if userMsg["content"] != "안녕하세요." {
		t.Fatalf("unexpected user message content: %v", userMsg["content"])
	}
}

// assertOllamaResponse 는 응답이 assistant 역할의 text 블록으로 환원됐는지 검증한다.
func assertOllamaResponse(t *testing.T, resp ChatResponse) {
	t.Helper()

	if resp.Message.Role != message.RoleAssistant {
		t.Fatalf("expected assistant role, got %q", resp.Message.Role)
	}
	if len(resp.Message.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Message.Content))
	}
	block := resp.Message.Content[0]
	if block.Type != message.BlockTypeText {
		t.Fatalf("expected text block, got %q", block.Type)
	}
	if block.Text != "안녕하세요, 무엇을 도와드릴까요?" {
		t.Fatalf("unexpected response text: %q", block.Text)
	}
}
