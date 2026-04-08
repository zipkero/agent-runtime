package planner_test

import (
	"context"
	"encoding/json"
	"testing"

	"agentflow/internal/planner"
	"agentflow/internal/state"
	"agentflow/internal/tools"
	"agentflow/internal/types"
	"agentflow/testutil"
)

// stubTool 은 registry 등록용 최소 Tool 구현체다.
type stubTool struct{ name string }

func (s *stubTool) Name() string              { return s.name }
func (s *stubTool) Description() string       { return "stub" }
func (s *stubTool) InputSchema() tools.Schema { return tools.Schema{} }
func (s *stubTool) Execute(_ context.Context, _ map[string]any) (types.ToolResult, error) {
	return types.ToolResult{}, nil
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

func newRegistry(toolNames ...string) tools.ToolRegistry {
	reg := tools.NewInMemoryToolRegistry()
	for _, name := range toolNames {
		reg.Register(&stubTool{name: name})
	}
	return reg
}

func baseState() state.AgentState {
	return state.AgentState{UserInput: "테스트 입력"}
}

// --- 성공 케이스 ---

func TestLLMPlanner_ValidRespondDirectly(t *testing.T) {
	expected := types.PlanResult{
		ActionType: types.ActionRespondDirectly,
		Reasoning:  "직접 응답",
	}
	mock := testutil.NewMockLLMClient().WithResponse(mustMarshal(t, expected))
	p := planner.NewLLMPlanner(mock, newRegistry())

	result, err := p.Plan(context.Background(), baseState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ActionType != types.ActionRespondDirectly {
		t.Errorf("action_type = %q, want %q", result.ActionType, types.ActionRespondDirectly)
	}
	if mock.CallCount() != 1 {
		t.Errorf("callCount = %d, want 1", mock.CallCount())
	}
}

func TestLLMPlanner_ValidToolCall(t *testing.T) {
	expected := types.PlanResult{
		ActionType: types.ActionToolCall,
		ToolName:   "search",
		ToolInput:  map[string]any{"query": "Go test"},
		Reasoning:  "검색 필요",
	}
	mock := testutil.NewMockLLMClient().WithResponse(mustMarshal(t, expected))
	p := planner.NewLLMPlanner(mock, newRegistry("search"))

	result, err := p.Plan(context.Background(), baseState())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ActionType != types.ActionToolCall {
		t.Errorf("action_type = %q, want %q", result.ActionType, types.ActionToolCall)
	}
	if result.ToolName != "search" {
		t.Errorf("tool_name = %q, want %q", result.ToolName, "search")
	}
	if mock.CallCount() != 1 {
		t.Errorf("callCount = %d, want 1", mock.CallCount())
	}
}

// --- invalid JSON 재시도 케이스 ---

func TestLLMPlanner_InvalidJSON_RetrySucceeds(t *testing.T) {
	valid := types.PlanResult{
		ActionType: types.ActionRespondDirectly,
		Reasoning:  "재시도 성공",
	}
	mock := testutil.NewMockLLMClient().
		WithResponse("not valid json {{{").
		WithResponse(mustMarshal(t, valid))
	p := planner.NewLLMPlanner(mock, newRegistry())

	result, err := p.Plan(context.Background(), baseState())
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if result.ActionType != types.ActionRespondDirectly {
		t.Errorf("action_type = %q, want %q", result.ActionType, types.ActionRespondDirectly)
	}
	if mock.CallCount() != 2 {
		t.Errorf("callCount = %d, want 2 (initial + retry)", mock.CallCount())
	}
}

func TestLLMPlanner_InvalidJSON_RetryAlsoFails(t *testing.T) {
	mock := testutil.NewMockLLMClient().
		WithResponse("not valid json {{{").
		WithResponse("still not valid ~~~")
	p := planner.NewLLMPlanner(mock, newRegistry())

	_, err := p.Plan(context.Background(), baseState())
	if err == nil {
		t.Fatal("expected error after both attempts fail, got nil")
	}
	if mock.CallCount() != 2 {
		t.Errorf("callCount = %d, want 2", mock.CallCount())
	}
}

// --- hallucination 방어 케이스 ---

func TestLLMPlanner_HallucinatedTool_RetrySucceeds(t *testing.T) {
	hallucinated := types.PlanResult{
		ActionType: types.ActionToolCall,
		ToolName:   "nonexistent_tool",
		Reasoning:  "없는 tool",
	}
	valid := types.PlanResult{
		ActionType: types.ActionToolCall,
		ToolName:   "search",
		Reasoning:  "재시도 후 올바른 tool",
	}
	mock := testutil.NewMockLLMClient().
		WithResponse(mustMarshal(t, hallucinated)).
		WithResponse(mustMarshal(t, valid))
	p := planner.NewLLMPlanner(mock, newRegistry("search"))

	result, err := p.Plan(context.Background(), baseState())
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if result.ToolName != "search" {
		t.Errorf("tool_name = %q, want %q", result.ToolName, "search")
	}
	if mock.CallCount() != 2 {
		t.Errorf("callCount = %d, want 2 (initial + retry)", mock.CallCount())
	}
}

func TestLLMPlanner_HallucinatedTool_RetryAlsoFails(t *testing.T) {
	hallucinated := types.PlanResult{
		ActionType: types.ActionToolCall,
		ToolName:   "nonexistent_tool",
		Reasoning:  "없는 tool",
	}
	mock := testutil.NewMockLLMClient().
		WithResponse(mustMarshal(t, hallucinated)).
		WithResponse(mustMarshal(t, hallucinated))
	p := planner.NewLLMPlanner(mock, newRegistry("search"))

	_, err := p.Plan(context.Background(), baseState())
	if err == nil {
		t.Fatal("expected error for hallucinated tool, got nil")
	}
	if mock.CallCount() != 2 {
		t.Errorf("callCount = %d, want 2", mock.CallCount())
	}
}
