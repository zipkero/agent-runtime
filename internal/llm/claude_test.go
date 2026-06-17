package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/message"
)

func TestClaudeClientMapsRequestAndResponse(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected configured api key header")
		}

		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_test",
			"type": "message",
			"role": "assistant",
			"model": "claude-test",
			"content": [
				{"type": "text", "text": "I will use a tool."},
				{"type": "tool_use", "id": "toolu_test", "name": "lookup", "input": {"query": "agent runtime"}}
			],
			"stop_reason": "tool_use",
			"stop_sequence": "",
			"usage": {"input_tokens": 1, "output_tokens": 2}
		}`))
	}))
	defer server.Close()

	client, err := newClaudeClient(testClaudeConfig(), claudeClientOptions{
		baseURL:    server.URL,
		httpClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("newClaudeClient returned error: %v", err)
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleSystem,
				Content: []message.ContentBlock{message.NewTextBlock("Be concise.")},
			},
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("Find this.")},
			},
			{
				Role: message.RoleTool,
				Content: []message.ContentBlock{
					message.NewToolResultBlock(message.ToolResult{
						ToolCallID: "toolu_previous",
						Content:    "previous result",
						IsError:    false,
					}),
				},
			},
		},
		Tools: []message.ToolSpec{
			{
				Name:        "lookup",
				Description: "Look up a query.",
				InputSchema: json.RawMessage(`{
					"type": "object",
					"properties": {"query": {"type": "string"}},
					"required": ["query"],
					"additionalProperties": false
				}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if !resp.Message.HasToolCalls() {
		t.Fatal("expected mapped tool call response")
	}
	if len(resp.Message.Content) != 2 {
		t.Fatalf("expected 2 response blocks, got %d", len(resp.Message.Content))
	}
	if resp.Message.Content[0].Text != "I will use a tool." {
		t.Errorf("unexpected response text: %q", resp.Message.Content[0].Text)
	}

	toolCall := resp.Message.Content[1].ToolCall
	if toolCall == nil {
		t.Fatal("expected response tool call")
	}
	if toolCall.ID != "toolu_test" || toolCall.Name != "lookup" {
		t.Fatalf("unexpected tool call: %+v", toolCall)
	}
	var toolInput map[string]any
	if err := json.Unmarshal(toolCall.Input, &toolInput); err != nil {
		t.Fatalf("decode tool input: %v", err)
	}
	if toolInput["query"] != "agent runtime" {
		t.Errorf("unexpected tool input: %s", toolCall.Input)
	}

	assertClaudeRequest(t, requestBody)
}

func TestClaudeClientUsesRequestModelOverride(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_test",
			"type": "message",
			"role": "assistant",
			"model": "override-model",
			"content": [{"type": "text", "text": "ok"}],
			"stop_reason": "end_turn",
			"stop_sequence": "",
			"usage": {"input_tokens": 1, "output_tokens": 1}
		}`))
	}))
	defer server.Close()

	client, err := newClaudeClient(testClaudeConfig(), claudeClientOptions{
		baseURL:    server.URL,
		httpClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("newClaudeClient returned error: %v", err)
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

func TestClaudeClientReturnsDeadlineError(t *testing.T) {
	client, err := newClaudeClient(testClaudeConfig(), claudeClientOptions{})
	if err != nil {
		t.Fatalf("newClaudeClient returned error: %v", err)
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

func TestClaudeClientReturnsAuthenticationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid api key"}}`))
	}))
	defer server.Close()

	client, err := newClaudeClient(testClaudeConfig(), claudeClientOptions{
		baseURL:    server.URL,
		httpClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("newClaudeClient returned error: %v", err)
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
		t.Fatal("expected authentication error")
	}
	if !strings.Contains(err.Error(), "claude authentication failed") {
		t.Fatalf("expected observable authentication error, got %v", err)
	}
}

func assertClaudeRequest(t *testing.T, requestBody map[string]any) {
	t.Helper()

	if requestBody["model"] != "claude-test" {
		t.Fatalf("unexpected model: %v", requestBody["model"])
	}
	if requestBody["max_tokens"] != float64(defaultClaudeMaxTokens) {
		t.Fatalf("unexpected max_tokens: %v", requestBody["max_tokens"])
	}

	system, ok := requestBody["system"].([]any)
	if !ok || len(system) != 1 {
		t.Fatalf("unexpected system blocks: %#v", requestBody["system"])
	}
	systemBlock, ok := system[0].(map[string]any)
	if !ok || systemBlock["text"] != "Be concise." {
		t.Fatalf("unexpected system block: %#v", system[0])
	}

	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected messages: %#v", requestBody["messages"])
	}

	userMessage, ok := messages[0].(map[string]any)
	if !ok || userMessage["role"] != "user" {
		t.Fatalf("unexpected user message: %#v", messages[0])
	}

	toolMessage, ok := messages[1].(map[string]any)
	if !ok || toolMessage["role"] != "user" {
		t.Fatalf("unexpected tool result message: %#v", messages[1])
	}
	toolContent, ok := toolMessage["content"].([]any)
	if !ok || len(toolContent) != 1 {
		t.Fatalf("unexpected tool content: %#v", toolMessage["content"])
	}
	toolResult, ok := toolContent[0].(map[string]any)
	if !ok || toolResult["type"] != "tool_result" || toolResult["tool_use_id"] != "toolu_previous" {
		t.Fatalf("unexpected tool_result block: %#v", toolContent[0])
	}

	tools, ok := requestBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("unexpected tools: %#v", requestBody["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["name"] != "lookup" || tool["description"] != "Look up a query." {
		t.Fatalf("unexpected tool: %#v", tools[0])
	}
	inputSchema, ok := tool["input_schema"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected input_schema: %#v", tool["input_schema"])
	}
	if inputSchema["additionalProperties"] != false {
		t.Fatalf("expected additionalProperties to be preserved, got %#v", inputSchema)
	}
}

func testClaudeConfig() config.Config {
	return config.Config{
		Provider: config.ProviderClaude,
		APIKey:   "test-key",
		Model:    "claude-test",
		Timeout:  time.Second,
	}
}
