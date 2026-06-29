package message

import "encoding/json"

// Role 은 Runtime 내부에서 사용하는 메시지 발신자 역할이다.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall 은 provider 응답에서 온 tool 호출 요청을 Runtime 내부 표현으로 보존한다.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolResult 는 Runtime이 실행한 tool 결과를 다음 LLM 입력으로 전달하기 위한 메시지 내용이다.
type ToolResult struct {
	ToolCallID string
	Name       string
	Content    string
	IsError    bool
}

// Message 는 LLM과 Runtime 사이에서 주고받는 provider-neutral 메시지 표현이다.
type Message struct {
	Role       Role
	Text       string
	ToolCalls  []ToolCall
	ToolResult *ToolResult
}

func System(text string) Message {
	return Message{Role: RoleSystem, Text: text}
}

func User(text string) Message {
	return Message{Role: RoleUser, Text: text}
}

func Assistant(text string, toolCalls ...ToolCall) Message {
	return Message{Role: RoleAssistant, Text: text, ToolCalls: toolCalls}
}

func Tool(result ToolResult) Message {
	return Message{Role: RoleTool, ToolResult: &result}
}
