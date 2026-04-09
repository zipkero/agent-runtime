package types

// ToolResult 는 Tool 실행 결과를 담는 구조체다.
// IsError 가 true 일 때 ErrMsg 에 에러 내용이 담긴다.
// Kind 는 WorkingMemory 분류 키로 사용되며, 각 Tool 패키지가 자율적으로 정의한다.
type ToolResult struct {
	ToolName string
	Kind     string
	Output   string
	IsError  bool
	ErrMsg   string
}
