// Package message는 LLM 공급자와 Agent Runtime이 공유하는 중립 메시지 모델을 정의한다.
package message

import "encoding/json"

// Role 타입은 Runtime 내부에서 사용하는 메시지 발신자 역할이다.
type Role string

const (
	// RoleSystem 상수는 공급자 요청의 시스템 지시문에 해당하는 역할이다.
	RoleSystem Role = "system"
	// RoleUser 상수는 호출자가 넣은 입력에 해당하는 역할이다.
	RoleUser Role = "user"
	// RoleAssistant 상수는 텍스트와 Tool 호출 요청을 담는 모델 응답 역할이다.
	RoleAssistant Role = "assistant"
	// RoleTool 상수는 Runtime이 실행한 Tool 결과를 담는 역할이며 내용은 ToolResult에 있다.
	RoleTool Role = "tool"
)

// ToolCall 구조체는 공급자 응답의 Tool 호출 요청을 Runtime 내부 표현으로 보존한다.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolResult 구조체는 Runtime이 실행한 Tool 결과를 다음 LLM 입력으로 전달하기 위한 메시지 내용이다.
type ToolResult struct {
	ToolCallID string
	Name       string
	Content    string
	IsError    bool
}

// ToolSchema 구조체는 LLM 요청 경계에 전달할 공급자 중립 Tool 계약이다.
type ToolSchema struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// Message 구조체는 LLM과 Runtime 사이에서 주고받는 공급자 중립 메시지 표현이다.
type Message struct {
	Role       Role
	Text       string
	ToolCalls  []ToolCall
	ToolResult *ToolResult
}

// System 함수는 시스템 지시문을 담은 메시지를 만든다.
func System(text string) Message {
	return Message{Role: RoleSystem, Text: text}
}

// User 함수는 사용자 입력을 담은 메시지를 만든다.
func User(text string) Message {
	return Message{Role: RoleUser, Text: text}
}

// Assistant 함수는 텍스트와 선택적 Tool 호출을 담은 Assistant 메시지를 만든다.
func Assistant(text string, toolCalls ...ToolCall) Message {
	return Message{Role: RoleAssistant, Text: text, ToolCalls: toolCalls}
}

// Tool 함수는 실행 결과를 담은 Tool 메시지를 만든다.
func Tool(result ToolResult) Message {
	return Message{Role: RoleTool, ToolResult: &result}
}
