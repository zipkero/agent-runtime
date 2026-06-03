package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
)

func TestStubClientReturnsTextResponse(t *testing.T) {
	client := llm.NewStubClient(llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewTextBlock("hello from stub"),
			},
		},
	})

	resp, err := chat(context.Background(), client)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp.Message.HasToolCalls() {
		t.Fatal("expected text response without tool calls")
	}
	if len(resp.Message.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Message.Content))
	}
	if resp.Message.Content[0].Type != message.BlockTypeText {
		t.Errorf("expected text block, got %q", resp.Message.Content[0].Type)
	}
	if resp.Message.Content[0].Text != "hello from stub" {
		t.Errorf("unexpected text response: %q", resp.Message.Content[0].Text)
	}
}

func TestStubClientReturnsToolCallResponse(t *testing.T) {
	client := llm.NewStubClient(llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_stub",
					Name:  "lookup",
					Input: json.RawMessage(`{"query":"agent runtime"}`),
				}),
			},
		},
	})

	resp, err := chat(context.Background(), client)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if !resp.Message.HasToolCalls() {
		t.Fatal("expected tool call response")
	}
	if len(resp.Message.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Message.Content))
	}

	block := resp.Message.Content[0]
	if block.Type != message.BlockTypeToolCall {
		t.Errorf("expected tool_call block, got %q", block.Type)
	}
	if block.ToolCall == nil {
		t.Fatal("expected ToolCall to be non-nil")
	}
	if block.ToolCall.ID != "call_stub" {
		t.Errorf("unexpected tool call ID: %q", block.ToolCall.ID)
	}
	if block.ToolCall.Name != "lookup" {
		t.Errorf("unexpected tool call name: %q", block.ToolCall.Name)
	}
}

func TestStubClientReturnsDeadlineError(t *testing.T) {
	client := llm.NewStubClient(llm.ChatResponse{
		Message: message.Message{
			Role:    message.RoleAssistant,
			Content: []message.ContentBlock{message.NewTextBlock("unused")},
		},
	})

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := chat(ctx, client)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded error, got %v", err)
	}
}

func chat(ctx context.Context, client llm.LLMClient) (llm.ChatResponse, error) {
	return client.Chat(ctx, llm.ChatRequest{
		Model: "stub-model",
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("hello")},
			},
		},
	})
}
