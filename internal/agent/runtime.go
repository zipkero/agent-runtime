package agent

import (
	"context"
	"fmt"

	"agentflow/internal/executor"
	"agentflow/internal/planner"
	"agentflow/internal/state"
	"agentflow/internal/types"
)

// Runtime 은 plan → execute → state 반영 → finish 판단 루프를 실행하는 조율자다.
type Runtime struct {
	Planner  planner.Planner
	Executor executor.Executor
	MaxStep  int
}

// Run 은 초기 AgentState 를 받아 finish 조건이 충족될 때까지 루프를 실행하고,
// 최종 AgentState 와 에러를 반환한다.
// ctx 취소 시 루프를 즉시 중단하고 현재 state 를 반환한다.
func (r *Runtime) Run(ctx context.Context, s state.AgentState) (state.AgentState, error) {
	for {
		// ctx 취소 확인
		select {
		case <-ctx.Done():
			s.Status = state.StatusFailed
			return s, ctx.Err()
		default:
		}

		// 1. Plan
		plan, err := r.Planner.Plan(ctx, s)
		if err != nil {
			s.Status = state.StatusFailed
			return s, fmt.Errorf("planner: %w", err)
		}

		// 2. respond_directly 이면 FinalAnswer 를 먼저 채운다.
		//    IsFinished 가 plan 반영 전 state 를 기준으로 판단하므로
		//    respond_directly + FinalAnswer 케이스는 여기서 채운 뒤 검사한다.
		if plan.ActionType == types.ActionRespondDirectly {
			s.FinalAnswer = plan.Reasoning
		}

		// 3. Finish 판단
		result := IsFinished(plan, s, r.MaxStep)
		if result.Finished {
			s.Status = state.StatusFinished
			return s, nil
		}

		// 4. Execute
		toolResult, err := r.Executor.Execute(ctx, plan)
		if err != nil {
			s.Status = state.StatusFailed
			return s, fmt.Errorf("executor: %w", err)
		}

		// 5. State 반영
		s.CurrentPlan = plan
		s.LastToolCall = plan.ToolName
		s.ToolResults = append(s.ToolResults, toolResult)
		s.StepCount++
	}
}
