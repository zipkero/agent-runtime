package planner

import (
	"context"

	"github.com/zipkero/gogent/internal/state"
	"github.com/zipkero/gogent/internal/types"
)

// MockPlanner 는 미리 정의된 PlanResult 목록을 순서대로 반환하는 테스트용 Planner 다.
// 목록을 모두 소진하면 ActionFinish 를 반환해 loop 를 종료한다.
type MockPlanner struct {
	Steps []types.PlanResult
	idx   int
}

func NewMockPlanner(steps []types.PlanResult) *MockPlanner {
	return &MockPlanner{Steps: steps}
}

func (m *MockPlanner) Plan(_ context.Context, _ state.AgentState) (types.PlanResult, error) {
	if m.idx >= len(m.Steps) {
		return types.PlanResult{ActionType: types.ActionFinish}, nil
	}
	r := m.Steps[m.idx]
	m.idx++
	return r, nil
}
