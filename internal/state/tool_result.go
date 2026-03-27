package state

// ToolResult 는 Tool 실행 결과를 담는 구조체다.
// IsError 가 true 일 때 ErrMsg 에 에러 내용이 담긴다.
type ToolResult struct {
	ToolName string
	Output   string
	IsError  bool
	ErrMsg   string
}
