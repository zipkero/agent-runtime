package state

import "agentflow/internal/types"

// AgentState 는 Agent Loop 전체가 공유하는 단일 상태 구조체다.
// Planner, Executor, Runtime 모두 이 구조체를 통해 상태를 주고받는다.
// Phase 4에서 RequestState / SessionState / WorkingMemory 로 분리될 예정이다.
type AgentState struct {
	RequestID    string
	SessionID    string
	UserInput    string
	LastToolCall string
	ToolResults  []types.ToolResult
	FinalAnswer  string
	StepCount    int
	Status       AgentStatus
	CurrentPlan  types.PlanResult
}
