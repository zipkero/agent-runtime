package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/zipkero/gogent/internal/executor"
	"github.com/zipkero/gogent/internal/planner"
	"github.com/zipkero/gogent/internal/state"
	"github.com/zipkero/gogent/internal/types"
)

func newRuntime(p planner.Planner, e executor.Executor, maxStep int) *Runtime {
	return &Runtime{Planner: p, Executor: e, MaxStep: maxStep}
}

func initialState() state.AgentState {
	return state.AgentState{
		Request: state.RequestState{
			RequestID: "req-1",
			UserInput: "test input",
		},
		Session: &state.SessionState{
			SessionID: "sess-1",
		},
		Status: state.StatusRunning,
	}
}

// tool_call 1회 후 ActionFinish 로 종료되는 케이스
func TestRun_ToolCallThenFinish(t *testing.T) {
	p := planner.NewMockPlanner([]types.PlanResult{
		{ActionType: types.ActionToolCall, ToolName: "search", ToolInput: map[string]any{"q": "go"}},
		{ActionType: types.ActionFinish},
	})
	e := executor.NewMockExecutor([]types.ToolResult{
		{ToolName: "search", Output: "result1"},
	})
	rt := newRuntime(p, e, 10)

	got, err := rt.Run(context.Background(), initialState())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != state.StatusFinished {
		t.Errorf("status = %q, want %q", got.Status, state.StatusFinished)
	}
	if got.StepCount != 1 {
		t.Errorf("StepCount = %d, want 1", got.StepCount)
	}
	if len(got.Request.ToolResults) != 1 || got.Request.ToolResults[0].Output != "result1" {
		t.Errorf("ToolResults = %v, want [{search result1}]", got.Request.ToolResults)
	}
	if got.LastToolCall != "search" {
		t.Errorf("LastToolCall = %q, want %q", got.LastToolCall, "search")
	}
}

// MaxStep 초과로 강제 종료되는 케이스
func TestRun_MaxStepExceeded(t *testing.T) {
	// tool_call 만 계속 반환 — MockPlanner 소진 후 ActionFinish 이지만 MaxStep=2 가 먼저
	p := planner.NewMockPlanner([]types.PlanResult{
		{ActionType: types.ActionToolCall, ToolName: "tool"},
		{ActionType: types.ActionToolCall, ToolName: "tool"},
		{ActionType: types.ActionToolCall, ToolName: "tool"},
	})
	e := executor.NewMockExecutor([]types.ToolResult{
		{ToolName: "tool", Output: "r1"},
		{ToolName: "tool", Output: "r2"},
		{ToolName: "tool", Output: "r3"},
	})
	rt := newRuntime(p, e, 2)

	got, err := rt.Run(context.Background(), initialState())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != state.StatusFinished {
		t.Errorf("status = %q, want %q", got.Status, state.StatusFinished)
	}
	if got.StepCount != 2 {
		t.Errorf("StepCount = %d, want 2", got.StepCount)
	}
}

// respond_directly 로 FinalAnswer 가 세팅되고 종료되는 케이스
func TestRun_RespondDirectly(t *testing.T) {
	const answer = "42"
	p := planner.NewMockPlanner([]types.PlanResult{
		{ActionType: types.ActionRespondDirectly, Reasoning: answer},
	})
	e := executor.NewMockExecutor(nil)
	rt := newRuntime(p, e, 10)

	got, err := rt.Run(context.Background(), initialState())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != state.StatusFinished {
		t.Errorf("status = %q, want %q", got.Status, state.StatusFinished)
	}
	if got.FinalAnswer != answer {
		t.Errorf("FinalAnswer = %q, want %q", got.FinalAnswer, answer)
	}
	if got.StepCount != 0 {
		t.Errorf("StepCount = %d, want 0 (respond_directly 는 execute 없이 종료)", got.StepCount)
	}
}

// ask_user 발생 시 FinalAnswer 채우고 즉시 종료되는 케이스
func TestRun_AskUser(t *testing.T) {
	const question = "어떤 날짜 범위로 검색할까요?"
	p := planner.NewMockPlanner([]types.PlanResult{
		{ActionType: types.ActionAskUser, Reasoning: question},
	})
	e := executor.NewMockExecutor(nil)
	rt := newRuntime(p, e, 10)

	got, err := rt.Run(context.Background(), initialState())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != state.StatusWaitingInput {
		t.Errorf("status = %q, want %q", got.Status, state.StatusWaitingInput)
	}
	if got.FinalAnswer != question {
		t.Errorf("FinalAnswer = %q, want %q", got.FinalAnswer, question)
	}
	// ask_user 는 Execute 를 호출하지 않으므로 StepCount 는 0 이어야 한다.
	if got.StepCount != 0 {
		t.Errorf("StepCount = %d, want 0", got.StepCount)
	}
}

// ctx 취소 시 StatusFailed + error 반환 케이스
func TestRun_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	p := planner.NewMockPlanner([]types.PlanResult{
		{ActionType: types.ActionToolCall, ToolName: "tool"},
	})
	e := executor.NewMockExecutor(nil)
	rt := newRuntime(p, e, 10)

	got, err := rt.Run(ctx, initialState())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if got.Status != state.StatusFailed {
		t.Errorf("status = %q, want %q", got.Status, state.StatusFailed)
	}
}
