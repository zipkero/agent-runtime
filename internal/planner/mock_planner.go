package planner

import (
	"context"

	"agentflow/internal/state"
)

// MockPlanner 는 미리 정의된 PlanResult 목록을 순서대로 반환하는 테스트용 Planner 다.
// 목록을 모두 소진하면 ActionFinish 를 반환해 loop 를 종료한다.
type MockPlanner struct {
	Steps []PlanResult
	idx   int
}

func NewMockPlanner(steps []PlanResult) *MockPlanner {
	return &MockPlanner{Steps: steps}
}

func (m *MockPlanner) Plan(_ context.Context, _ state.AgentState) (PlanResult, error) {
	if m.idx >= len(m.Steps) {
		return PlanResult{ActionType: ActionFinish}, nil
	}
	r := m.Steps[m.idx]
	m.idx++
	return r, nil
}
