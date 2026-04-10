package executor

import (
	"context"

	"github.com/zipkero/gogent/internal/types"
)

// MockExecutor 는 미리 정의된 ToolResult 목록을 순서대로 반환하는 테스트용 Executor 다.
// 목록을 모두 소진하면 Output 이 빈 ToolResult 를 반환한다.
type MockExecutor struct {
	Results []types.ToolResult
	idx     int
}

func NewMockExecutor(results []types.ToolResult) *MockExecutor {
	return &MockExecutor{Results: results}
}

func (m *MockExecutor) Execute(_ context.Context, plan types.PlanResult) (types.ToolResult, error) {
	if m.idx >= len(m.Results) {
		return types.ToolResult{ToolName: plan.ToolName}, nil
	}
	r := m.Results[m.idx]
	m.idx++
	return r, nil
}
