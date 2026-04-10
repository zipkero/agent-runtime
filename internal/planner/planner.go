package planner

import (
	"context"

	"github.com/zipkero/gogent/internal/state"
	"github.com/zipkero/gogent/internal/types"
)

// Planner 는 현재 AgentState 를 보고 다음 행동(PlanResult)을 결정하는 인터페이스다.
// "무엇을 할지"만 결정하며, 실제 실행은 Executor 가 담당한다.
type Planner interface {
	Plan(ctx context.Context, s state.AgentState) (types.PlanResult, error)
}
