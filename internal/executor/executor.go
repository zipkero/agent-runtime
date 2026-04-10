package executor

import (
	"context"

	"github.com/zipkero/gogent/internal/types"
)

// Executor 는 PlanResult 를 실제 행동으로 연결하고 ToolResult 를 반환하는 인터페이스다.
// "어떻게 실행할지"를 담당하며, 다음 행동 결정은 Planner 의 역할이다.
type Executor interface {
	Execute(ctx context.Context, plan types.PlanResult) (types.ToolResult, error)
}
