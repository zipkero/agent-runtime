package message_test

import (
	"encoding/json"
	"testing"

	"github.com/zipkero/agent-runtime/internal/message"
)

// TestTextOnlyMessage 는 text-only assistant 메시지가 텍스트 응답(tool call 없음)으로
// 올바르게 판별되는지 검증한다.
func TestTextOnlyMessage(t *testing.T) {
	msg := message.Message{
		Role: message.RoleAssistant,
		Content: []message.ContentBlock{
			message.NewTextBlock("Hello, world!"),
		},
	}

	if msg.HasToolCalls() {
		t.Error("expected no tool calls in a text-only message")
	}

	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(msg.Content))
	}

	block := msg.Content[0]
	if block.Type != message.BlockTypeText {
		t.Errorf("expected block type %q, got %q", message.BlockTypeText, block.Type)
	}
	if block.Text != "Hello, world!" {
		t.Errorf("unexpected text: %q", block.Text)
	}
}

// TestToolCallMessage 는 tool_call 블록을 가진 assistant 메시지가 tool call 응답으로
// 올바르게 판별되는지 검증한다.
func TestToolCallMessage(t *testing.T) {
	inputJSON := json.RawMessage(`{"query":"weather in Seoul"}`)

	msg := message.Message{
		Role: message.RoleAssistant,
		Content: []message.ContentBlock{
			message.NewToolCallBlock(message.ToolCall{
				ID:    "call_abc123",
				Name:  "search",
				Input: inputJSON,
			}),
		},
	}

	if !msg.HasToolCalls() {
		t.Error("expected HasToolCalls to return true for a message with a tool_call block")
	}

	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(msg.Content))
	}

	block := msg.Content[0]
	if block.Type != message.BlockTypeToolCall {
		t.Errorf("expected block type %q, got %q", message.BlockTypeToolCall, block.Type)
	}
	if block.ToolCall == nil {
		t.Fatal("expected ToolCall to be non-nil")
	}
	if block.ToolCall.ID != "call_abc123" {
		t.Errorf("unexpected tool call ID: %q", block.ToolCall.ID)
	}
	if block.ToolCall.Name != "search" {
		t.Errorf("unexpected tool call name: %q", block.ToolCall.Name)
	}
}

// TestMixedContentMessage 는 text와 tool_call 블록을 함께 담은 메시지가 tool call 응답으로
// 판별되고 두 블록을 모두 보존하는지 검증한다.
func TestMixedContentMessage(t *testing.T) {
	msg := message.Message{
		Role: message.RoleAssistant,
		Content: []message.ContentBlock{
			message.NewTextBlock("Let me look that up."),
			message.NewToolCallBlock(message.ToolCall{
				ID:    "call_xyz",
				Name:  "calculator",
				Input: json.RawMessage(`{"expression":"2+2"}`),
			}),
		},
	}

	if !msg.HasToolCalls() {
		t.Error("expected HasToolCalls to return true for a mixed-content message")
	}
	if len(msg.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != message.BlockTypeText {
		t.Errorf("expected first block to be text, got %q", msg.Content[0].Type)
	}
	if msg.Content[1].Type != message.BlockTypeToolCall {
		t.Errorf("expected second block to be tool_call, got %q", msg.Content[1].Type)
	}
}

// TestToolResultMessage 는 tool 역할 메시지가 tool_result 블록을 담을 수 있는지 검증한다.
func TestToolResultMessage(t *testing.T) {
	msg := message.Message{
		Role: message.RoleTool,
		Content: []message.ContentBlock{
			message.NewToolResultBlock(message.ToolResult{
				ToolCallID: "call_abc123",
				Content:    "Seoul: 25°C, sunny",
				IsError:    false,
			}),
		},
	}

	if msg.HasToolCalls() {
		t.Error("tool result message should not be identified as having tool calls")
	}

	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(msg.Content))
	}

	block := msg.Content[0]
	if block.Type != message.BlockTypeToolResult {
		t.Errorf("expected block type %q, got %q", message.BlockTypeToolResult, block.Type)
	}
	if block.ToolResult == nil {
		t.Fatal("expected ToolResult to be non-nil")
	}
	if block.ToolResult.ToolCallID != "call_abc123" {
		t.Errorf("unexpected ToolCallID: %q", block.ToolResult.ToolCallID)
	}
	if block.ToolResult.Content != "Seoul: 25°C, sunny" {
		t.Errorf("unexpected content: %q", block.ToolResult.Content)
	}
	if block.ToolResult.IsError {
		t.Error("expected IsError to be false")
	}
}

// TestUserAndSystemMessages 는 user·system 메시지를 구성할 수 있고 각 역할이 올바르게
// 보존되는지 검증한다.
func TestUserAndSystemMessages(t *testing.T) {
	user := message.Message{
		Role:    message.RoleUser,
		Content: []message.ContentBlock{message.NewTextBlock("What is the capital of France?")},
	}
	system := message.Message{
		Role:    message.RoleSystem,
		Content: []message.ContentBlock{message.NewTextBlock("You are a helpful assistant.")},
	}

	if user.Role != message.RoleUser {
		t.Errorf("expected user role, got %q", user.Role)
	}
	if system.Role != message.RoleSystem {
		t.Errorf("expected system role, got %q", system.Role)
	}
	if user.HasToolCalls() || system.HasToolCalls() {
		t.Error("text-only user/system messages should not have tool calls")
	}
}
