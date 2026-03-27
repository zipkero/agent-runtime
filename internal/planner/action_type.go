package planner

// ActionType 은 Planner가 결정할 수 있는 행동 유형이다.
type ActionType string

const (
	// ActionToolCall 은 특정 Tool을 호출하도록 지시한다.
	ActionToolCall ActionType = "tool_call"
	// ActionRespondDirectly 는 Tool 없이 바로 응답을 생성한다.
	ActionRespondDirectly ActionType = "respond_directly"
	// ActionFinish 는 loop를 종료한다.
	ActionFinish ActionType = "finish"
)
