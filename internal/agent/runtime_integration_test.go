package agent

import (
	"context"
	"encoding/json"
	"testing"

	"agentflow/internal/executor"
	"agentflow/internal/planner"
	"agentflow/internal/state"
	"agentflow/internal/tools"
	"agentflow/internal/tools/calculator"
	"agentflow/internal/types"
	"agentflow/testutil"
)

// mustMarshalResult 는 PlanResult 를 JSON 문자열로 직렬화한다.
func mustMarshalResult(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

// TestRun_EndToEnd_RealToolExecution 은 LLMPlanner(MockLLMClient) → Runtime →
// ToolExecutor(real) → ToolRouter(real) → calculator tool 까지의 end-to-end 체인을 검증한다.
// runtime_test.go 의 모든 케이스가 MockExecutor 를 사용하므로, 실제 ToolExecutor + ToolRouter
// 연결이 동작함을 확인하는 별도 테스트가 필요하다.
func TestRun_EndToEnd_RealToolExecution(t *testing.T) {
	toolCallResp := types.PlanResult{
		ActionType: types.ActionToolCall,
		ToolName:   "calculator",
		ToolInput:  map[string]any{"expression": "3 + 4"},
		Reasoning:  "calculator 로 계산",
	}
	finishResp := types.PlanResult{
		ActionType: types.ActionFinish,
	}

	mockLLM := testutil.NewMockLLMClient().
		WithResponse(mustMarshalResult(t, toolCallResp)).
		WithResponse(mustMarshalResult(t, finishResp))

	reg := tools.NewInMemoryToolRegistry()
	reg.Register(calculator.New())

	p := planner.NewLLMPlanner(mockLLM, reg)
	router := tools.NewToolRouter(reg)
	e := executor.NewToolExecutor(router)
	rt := NewRuntime(p, e, 10)

	s := state.AgentState{
		RequestID: "req-e2e",
		SessionID: "sess-e2e",
		UserInput: "3 더하기 4를 계산해줘",
		Status:    state.StatusRunning,
	}

	got, err := rt.Run(context.Background(), s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != state.StatusFinished {
		t.Errorf("status = %q, want %q", got.Status, state.StatusFinished)
	}
	if got.StepCount != 1 {
		t.Errorf("StepCount = %d, want 1", got.StepCount)
	}
	if len(got.ToolResults) != 1 {
		t.Fatalf("ToolResults len = %d, want 1", len(got.ToolResults))
	}
	if got.ToolResults[0].ToolName != "calculator" {
		t.Errorf("ToolResults[0].ToolName = %q, want %q", got.ToolResults[0].ToolName, "calculator")
	}
	if got.ToolResults[0].Output != "7" {
		t.Errorf("ToolResults[0].Output = %q, want %q", got.ToolResults[0].Output, "7")
	}
	if mockLLM.CallCount() != 2 {
		t.Errorf("LLM callCount = %d, want 2", mockLLM.CallCount())
	}
}
