package tools

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zipkero/gogent/internal/observability"
	"github.com/zipkero/gogent/internal/types"
)

// ToolRouter 는 PlanResult 를 받아 registry 에서 tool 을 조회하고 실행한다.
// planner 와 tool 구현체 사이를 중재하며, 에러를 유형별로 분류해 반환한다.
type ToolRouter struct {
	registry ToolRegistry
	logger   *slog.Logger
}

func NewToolRouter(registry ToolRegistry) *ToolRouter {
	return &ToolRouter{registry: registry, logger: observability.New()}
}

// Route 는 PlanResult 의 ToolName 으로 tool 을 조회하고 ToolInput 을 검증한 뒤 실행한다.
//
// 에러 유형:
//   - tool_not_found  : registry 에 없는 이름 → fatal
//   - input_validation_failed : required 필드 누락 또는 타입 불일치 → fatal
//   - tool_execution_failed   : Execute() 에서 error 반환 → retryable
func (r *ToolRouter) Route(ctx context.Context, plan types.PlanResult) (types.ToolResult, error) {
	start := time.Now()
	log := observability.FromContext(ctx, r.logger)

	tool, err := r.registry.Get(plan.ToolName)
	if err != nil {
		routeErr := types.NewToolNotFoundError(plan.ToolName)
		log.ErrorContext(ctx, "tool route failed",
			"tool_name", plan.ToolName,
			"error_kind", routeErr.Kind,
			"error", routeErr.Msg,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return types.ToolResult{}, routeErr
	}

	if err := validateInput(tool.InputSchema(), plan.ToolInput); err != nil {
		routeErr := types.NewInputValidationError(err.Error())
		log.ErrorContext(ctx, "tool route failed",
			"tool_name", plan.ToolName,
			"error_kind", routeErr.Kind,
			"error", routeErr.Msg,
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return types.ToolResult{}, routeErr
	}

	result, err := tool.Execute(ctx, plan.ToolInput)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		routeErr := types.NewToolExecutionError(plan.ToolName, err)
		log.ErrorContext(ctx, "tool route failed",
			"tool_name", plan.ToolName,
			"input", plan.ToolInput,
			"error_kind", routeErr.Kind,
			"error", routeErr.Msg,
			"duration_ms", duration,
		)
		return types.ToolResult{}, routeErr
	}

	log.InfoContext(ctx, "tool route succeeded",
		"tool_name", plan.ToolName,
		"input", plan.ToolInput,
		"output_summary", outputSummary(result),
		"is_error", result.IsError,
		"duration_ms", duration,
	)
	return result, nil
}

// outputSummary 는 ToolResult 출력을 100자 이내로 요약한다.
func outputSummary(r types.ToolResult) string {
	if r.IsError {
		return r.ErrMsg
	}
	const maxLen = 100
	if len(r.Output) <= maxLen {
		return r.Output
	}
	return r.Output[:maxLen] + "..."
}

// validateInput 은 schema 의 required 필드 존재 여부와 타입을 검증한다.
func validateInput(schema Schema, input map[string]any) error {
	for _, field := range schema.Fields {
		val, ok := input[field.Name]
		if !ok {
			if field.Required {
				return fmt.Errorf("required field %q is missing", field.Name)
			}
			continue
		}
		if err := checkType(field, val); err != nil {
			return err
		}
	}
	return nil
}

func checkType(field FieldSchema, val any) error {
	switch field.Type {
	case FieldTypeString:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("field %q must be string, got %T", field.Name, val)
		}
	case FieldTypeNumber:
		switch val.(type) {
		case int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64,
			float32, float64:
		default:
			return fmt.Errorf("field %q must be number, got %T", field.Name, val)
		}
	case FieldTypeBoolean:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("field %q must be boolean, got %T", field.Name, val)
		}
	}
	return nil
}
