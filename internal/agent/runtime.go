package agent

import (
	"context"
	"fmt"
	"log/slog"

	"agentflow/internal/executor"
	"agentflow/internal/observability"
	"agentflow/internal/planner"
	"agentflow/internal/state"
	"agentflow/internal/types"
)

// Runtime 은 plan → execute → state 반영 → finish 판단 루프를 실행하는 조율자다.
type Runtime struct {
	Planner  planner.Planner
	Executor executor.Executor
	MaxStep  int
	logger   *slog.Logger
}

// NewRuntime 은 Runtime 을 생성한다.
func NewRuntime(p planner.Planner, e executor.Executor, maxStep int) *Runtime {
	return &Runtime{
		Planner:  p,
		Executor: e,
		MaxStep:  maxStep,
		logger:   observability.New(),
	}
}

// Run 은 초기 AgentState 를 받아 finish 조건이 충족될 때까지 루프를 실행하고,
// 최종 AgentState 와 에러를 반환한다.
// ctx 취소 시 루프를 즉시 중단하고 현재 state 를 반환한다.
func (r *Runtime) Run(ctx context.Context, s state.AgentState) (state.AgentState, error) {
	base := r.logger
	if base == nil {
		base = observability.New()
	}
	log := observability.FromContext(ctx, base)

	for {
		// ctx 취소 확인
		select {
		case <-ctx.Done():
			s.Status = state.StatusFailed
			return s, ctx.Err()
		default:
		}

		log.InfoContext(ctx, "runtime step start",
			"step_count", s.StepCount,
			"last_tool_call", s.LastToolCall,
		)

		// 1. Plan
		plan, err := r.Planner.Plan(ctx, s)
		if err != nil {
			log.ErrorContext(ctx, "runtime plan failed",
				"step_count", s.StepCount,
				"error", err,
			)
			s.Status = state.StatusFailed
			return s, fmt.Errorf("planner: %w", err)
		}

		// 2. respond_directly / summarize 이면 FinalAnswer 를 먼저 채운다.
		//    IsFinished 가 plan 반영 전 state 를 기준으로 판단하므로
		//    FinalAnswer 를 여기서 채운 뒤 검사해야 즉시 종료된다.
		//    summarize 는 Executor 를 호출하지 않고 Reasoning 을 그대로 FinalAnswer 로 사용한다.
		if plan.ActionType == types.ActionRespondDirectly ||
			plan.ActionType == types.ActionSummarize ||
			plan.ActionType == types.ActionAskUser {
			s.FinalAnswer = plan.Reasoning
		}

		// 3. Finish 판단
		result := IsFinished(plan, s, r.MaxStep)
		if result.Finished {
			log.InfoContext(ctx, "runtime finished",
				"step_count", s.StepCount,
				"reason", result.Reason,
				"action_type", plan.ActionType,
			)
			s.Status = state.StatusFinished
			return s, nil
		}

		// 4. Execute
		toolResult, err := r.Executor.Execute(ctx, plan)
		if err != nil {
			log.ErrorContext(ctx, "runtime execute failed",
				"step_count", s.StepCount,
				"tool_name", plan.ToolName,
				"error", err,
			)
			s.Status = state.StatusFailed
			return s, fmt.Errorf("executor: %w", err)
		}

		// 5. State 반영
		s.CurrentPlan = plan
		s.LastToolCall = plan.ToolName
		s.ToolResults = append(s.ToolResults, toolResult)
		s.StepCount++

		log.InfoContext(ctx, "runtime step complete",
			"step_count", s.StepCount,
			"tool_name", plan.ToolName,
		)
	}
}
