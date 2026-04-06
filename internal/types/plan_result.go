package types

// ActionType 은 Planner 가 결정할 수 있는 행동 유형이다.
type ActionType string

const (
	// ActionToolCall 은 특정 Tool 을 호출하도록 지시한다.
	ActionToolCall ActionType = "tool_call"
	// ActionRespondDirectly 는 Tool 없이 바로 응답을 생성한다.
	ActionRespondDirectly ActionType = "respond_directly"
	// ActionFinish 는 loop 를 종료한다.
	ActionFinish ActionType = "finish"
	// ActionSummarize 는 지금까지의 ToolResults 를 요약해 응답을 생성한다.
	// Executor 를 호출하지 않고 respond_directly 와 동일하게 loop 를 종료한다.
	ActionSummarize ActionType = "summarize"
	// ActionAskUser 는 사용자에게 추가 입력을 요청한다.
	// CLI 환경에서는 FinalAnswer 에 질문 문자열을 채우고 loop 를 즉시 종료한다.
	// HTTP API 환경에서의 비동기 대기 메커니즘은 Phase 7 에서 구현한다.
	ActionAskUser ActionType = "ask_user"
)

// PlanResult 는 Planner 가 내린 결정을 담는 구조체다.
// ActionType 이 tool_call 일 때 ToolName 과 ToolInput 이 유효하다.
// ActionType 이 respond_directly 일 때 Reasoning 이 FinalAnswer 로 사용된다.
type PlanResult struct {
	ActionType ActionType
	ToolName   string
	ToolInput  map[string]any
	Reasoning  string
}
