package tools_test

import (
	"context"
	"testing"

	"github.com/zipkero/gogent/internal/tools"
	"github.com/zipkero/gogent/internal/tools/calculator"
	"github.com/zipkero/gogent/internal/types"
)

func newRouterWithCalc() *tools.ToolRouter {
	r := tools.NewInMemoryToolRegistry()
	r.Register(calculator.New())
	return tools.NewToolRouter(r)
}

func TestToolRouter_ValidTool(t *testing.T) {
	router := newRouterWithCalc()

	result, err := router.Route(context.Background(), types.PlanResult{
		ActionType: types.ActionToolCall,
		ToolName:   "calculator",
		ToolInput:  map[string]any{"expression": "2 + 3"},
	})

	if err != nil {
		t.Fatalf("예상치 못한 에러: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool 실행 실패: %s", result.ErrMsg)
	}
	if result.Output != "5" {
		t.Errorf("결과 불일치: got %q, want %q", result.Output, "5")
	}
}

func TestToolRouter_ToolNotFound(t *testing.T) {
	router := newRouterWithCalc()

	_, err := router.Route(context.Background(), types.PlanResult{
		ActionType: types.ActionToolCall,
		ToolName:   "nonexistent",
		ToolInput:  map[string]any{},
	})

	agentErr := assertAgentError(t, err)
	if agentErr.Kind != types.ErrToolNotFound {
		t.Errorf("에러 유형 불일치: got %q, want %q", agentErr.Kind, types.ErrToolNotFound)
	}
	if agentErr.Retryable {
		t.Error("tool_not_found 는 fatal 이어야 한다")
	}
}

func TestToolRouter_InputValidationFailed_MissingRequired(t *testing.T) {
	router := newRouterWithCalc()

	_, err := router.Route(context.Background(), types.PlanResult{
		ActionType: types.ActionToolCall,
		ToolName:   "calculator",
		ToolInput:  map[string]any{}, // expression 누락
	})

	agentErr := assertAgentError(t, err)
	if agentErr.Kind != types.ErrInputValidationFailed {
		t.Errorf("에러 유형 불일치: got %q, want %q", agentErr.Kind, types.ErrInputValidationFailed)
	}
	if agentErr.Retryable {
		t.Error("input_validation_failed 는 fatal 이어야 한다")
	}
}

func TestToolRouter_InputValidationFailed_TypeMismatch(t *testing.T) {
	router := newRouterWithCalc()

	_, err := router.Route(context.Background(), types.PlanResult{
		ActionType: types.ActionToolCall,
		ToolName:   "calculator",
		ToolInput:  map[string]any{"expression": 12345}, // string 이어야 하는데 int
	})

	agentErr := assertAgentError(t, err)
	if agentErr.Kind != types.ErrInputValidationFailed {
		t.Errorf("에러 유형 불일치: got %q, want %q", agentErr.Kind, types.ErrInputValidationFailed)
	}
	if agentErr.Retryable {
		t.Error("input_validation_failed 는 fatal 이어야 한다")
	}
}

// assertAgentError 는 err 가 *types.AgentError 타입인지 확인하고 반환한다.
func assertAgentError(t *testing.T, err error) *types.AgentError {
	t.Helper()
	if err == nil {
		t.Fatal("error 가 반환되어야 한다")
	}
	agentErr, ok := err.(*types.AgentError)
	if !ok {
		t.Fatalf("*types.AgentError 타입이어야 한다, got %T", err)
	}
	return agentErr
}
