package state

import "github.com/zipkero/agent-runtime/internal/types"

// AgentState 는 Agent Loop 전체가 공유하는 aggregator 상태 구조체다.
// 요청 범위(Request)와 세션 범위(Session)를 명시적으로 분리하고,
// loop 실행 중에만 유효한 필드(LastToolCall, FinalAnswer, StepCount, Status, CurrentPlan)를 직접 보유한다.
type AgentState struct {
	// Request 는 단일 Run() 호출 범위의 요청 데이터다.
	Request RequestState
	// Session 은 여러 Run() 호출을 넘어 지속되는 세션 데이터다.
	// nil 이면 세션 없음(anonymous 요청). 저장소에서 로드된 경우 non-nil.
	Session *SessionState
	// LastToolCall 은 직전 step 에서 실행된 tool 이름이다 (loop step 범위).
	LastToolCall string
	// FinalAnswer 는 loop 종료 시 사용자에게 반환할 최종 응답이다.
	FinalAnswer string
	// StepCount 는 현재까지 실행된 step 수다.
	StepCount int
	// Status 는 현재 loop 의 실행 상태다.
	Status AgentStatus
	// CurrentPlan 은 가장 최근 Planner 가 반환한 결정이다 (loop step 범위).
	CurrentPlan types.PlanResult
	// RelevantMemories 는 Run() 시작 시 LoadRelevantMemory 로 조회된 Long-term Memory 목록이다.
	// prompt_builder 가 이 필드를 읽어 system prompt 의 context 섹션에 포함한다.
	RelevantMemories []types.Memory
}
