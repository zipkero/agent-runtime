package message

import (
	"encoding/json"
	"testing"
)

// TestConstructorsPreserveMessageRolesAndText 는 생성자가 provider와 무관한 role/text contract를 보존하는지 확인한다.
func TestConstructorsPreserveMessageRolesAndText(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		role Role
		text string
	}{
		{name: "system", msg: System("runtime rule"), role: RoleSystem, text: "runtime rule"},
		{name: "user", msg: User("hello"), role: RoleUser, text: "hello"},
		{name: "assistant", msg: Assistant("answer"), role: RoleAssistant, text: "answer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg.Role != tt.role {
				t.Fatalf("Role = %q, want %q", tt.msg.Role, tt.role)
			}
			if tt.msg.Text != tt.text {
				t.Fatalf("Text = %q, want %q", tt.msg.Text, tt.text)
			}
		})
	}
}

// TestAssistantPreservesToolCalls 는 Assistant 메시지가 provider 응답의 tool call 정보를 잃지 않는지 확인한다.
func TestAssistantPreservesToolCalls(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md"}`)
	msg := Assistant("need tool", ToolCall{
		ID:        "call-1",
		Name:      "read_file",
		Arguments: args,
	})

	if len(msg.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(msg.ToolCalls))
	}
	call := msg.ToolCalls[0]
	if call.ID != "call-1" || call.Name != "read_file" {
		t.Fatalf("ToolCall = %+v, want id=call-1 name=read_file", call)
	}
	if string(call.Arguments) != string(args) {
		t.Fatalf("Arguments = %s, want %s", call.Arguments, args)
	}
}

// TestToolMessagePreservesToolResult 는 Tool 메시지가 이후 LLM 입력에 필요한 tool result 정보를 보존하는지 확인한다.
func TestToolMessagePreservesToolResult(t *testing.T) {
	msg := Tool(ToolResult{
		ToolCallID: "call-1",
		Name:       "read_file",
		Content:    "file body",
		IsError:    true,
	})

	if msg.Role != RoleTool {
		t.Fatalf("Role = %q, want %q", msg.Role, RoleTool)
	}
	if msg.ToolResult == nil {
		t.Fatal("ToolResult = nil, want value")
	}
	if msg.ToolResult.ToolCallID != "call-1" || msg.ToolResult.Name != "read_file" {
		t.Fatalf("ToolResult = %+v, want call-1/read_file", msg.ToolResult)
	}
	if msg.ToolResult.Content != "file body" || !msg.ToolResult.IsError {
		t.Fatalf("ToolResult = %+v, want content and error flag preserved", msg.ToolResult)
	}
}
