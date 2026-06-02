// Package message 는 runtime 전반에서 공유하는 provider-neutral 메시지 타입을 정의한다.
// 이 패키지는 다른 internal 패키지를 import하지 않는다.
package message

import "encoding/json"

// Role 은 메시지 참여자의 역할을 나타낸다.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

// BlockType 은 ContentBlock이 담은 content의 종류를 식별한다.
type BlockType string

const (
	BlockTypeText       BlockType = "text"
	BlockTypeToolCall   BlockType = "tool_call"
	BlockTypeToolResult BlockType = "tool_result"
)

// Message 는 대화 속 하나의 메시지다.
// 하나의 assistant 메시지는 text 블록과 tool_call 블록을 함께 담을 수 있다.
type Message struct {
	Role    Role
	Content []ContentBlock
}

// HasToolCalls 는 메시지에 tool_call 블록이 하나라도 있는지 여부를 반환한다.
func (m Message) HasToolCalls() bool {
	for _, b := range m.Content {
		if b.Type == BlockTypeToolCall {
			return true
		}
	}
	return false
}

// ContentBlock 은 메시지를 구성하는 하나의 content 조각이다.
// Type에 따라 Text, ToolCall, ToolResult 중 정확히 하나만 유효하다.
type ContentBlock struct {
	Type       BlockType
	Text       string      // Type == BlockTypeText일 때 유효
	ToolCall   *ToolCall   // Type == BlockTypeToolCall일 때 유효
	ToolResult *ToolResult // Type == BlockTypeToolResult일 때 유효
}

// NewTextBlock 은 text ContentBlock을 생성한다.
func NewTextBlock(text string) ContentBlock {
	return ContentBlock{
		Type: BlockTypeText,
		Text: text,
	}
}

// NewToolCallBlock 은 tool_call ContentBlock을 생성한다.
func NewToolCallBlock(tc ToolCall) ContentBlock {
	return ContentBlock{
		Type:     BlockTypeToolCall,
		ToolCall: &tc,
	}
}

// NewToolResultBlock 은 tool_result ContentBlock을 생성한다.
func NewToolResultBlock(tr ToolResult) ContentBlock {
	return ContentBlock{
		Type:       BlockTypeToolResult,
		ToolResult: &tr,
	}
}

// ToolCall 은 assistant가 요청한 tool 호출을 나타낸다.
// Input은 raw JSON으로 보관하며, 검증·실행은 Phase 3 범위다.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult 는 tool 호출의 결과를 나타낸다.
type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// ToolSpec 은 LLM에 제공할 수 있는 tool의 schema 정의다.
// runtime은 이를 chat 요청에 담아 tool call 생성을 유도한다.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage // tool의 입력 파라미터를 기술하는 JSON Schema
}
