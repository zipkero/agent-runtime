package planner

// PlanResult 는 Planner가 내린 결정을 담는 구조체다.
// ActionType 이 tool_call 일 때 ToolName 과 ToolInput 이 유효하다.
// ActionType 이 respond_directly 일 때 Reasoning 이 FinalAnswer 로 사용된다.
type PlanResult struct {
	ActionType ActionType
	ToolName   string
	ToolInput  map[string]any
	Reasoning  string
}
