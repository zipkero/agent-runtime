package executor

import (
	"context"
	"fmt"

	"github.com/zipkero/gogent/internal/tools"
	"github.com/zipkero/gogent/internal/types"
)

// ToolExecutor 는 ToolRouter 를 통해 실제 tool 을 실행하는 Executor 구현체다.
// PlanResult.ActionType 이 tool_call 이 아닌 경우 빈 ToolResult 를 반환한다.
type ToolExecutor struct {
	router *tools.ToolRouter
}

func NewToolExecutor(router *tools.ToolRouter) *ToolExecutor {
	return &ToolExecutor{router: router}
}

func (e *ToolExecutor) Execute(ctx context.Context, plan types.PlanResult) (types.ToolResult, error) {
	if plan.ActionType != types.ActionToolCall {
		return types.ToolResult{}, fmt.Errorf("ToolExecutor: unexpected action type %q — only tool_call is executable", plan.ActionType)
	}
	return e.router.Route(ctx, plan)
}
