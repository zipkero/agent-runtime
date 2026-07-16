package tool

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

// Calculator 구조체는 두 피연산자에 기본 사칙연산을 적용하는 내장 Tool이다.
type Calculator struct{}

// NewCalculator 함수는 상태를 갖지 않는 Calculator를 만든다.
func NewCalculator() Calculator {
	return Calculator{}
}

func (Calculator) Name() string {
	return "calculator"
}

func (Calculator) Description() string {
	return "Calculate a binary arithmetic expression."
}

func (Calculator) Schema() message.ToolSchema {
	return message.ToolSchema{
		Name:        "calculator",
		Description: "Calculate a binary arithmetic expression.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"left":{"type":"number"},"operator":{"type":"string","enum":["+","-","*","/"]},"right":{"type":"number"}},"required":["left","operator","right"],"additionalProperties":false}`),
	}
}

func (Calculator) Validate(args json.RawMessage) error {
	_, err := decodeCalculatorArguments(args)
	return err
}

func (Calculator) Execute(ctx context.Context, args json.RawMessage) (Result, error) {
	arguments, err := decodeCalculatorArguments(args)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, canceledExecutionError("calculator", err)
	}

	var value float64
	switch arguments.Operator {
	case "+":
		value = *arguments.Left + *arguments.Right
	case "-":
		value = *arguments.Left - *arguments.Right
	case "*":
		value = *arguments.Left * *arguments.Right
	case "/":
		if *arguments.Right == 0 {
			return Result{}, ExecutionErrorf("division by zero")
		}
		value = *arguments.Left / *arguments.Right
	default:
		return Result{}, ValidationErrorf("unsupported operator %q", arguments.Operator)
	}

	return Result{Content: strconv.FormatFloat(value, 'f', -1, 64)}, nil
}

type calculatorArguments struct {
	Left     *float64 `json:"left"`
	Operator string   `json:"operator"`
	Right    *float64 `json:"right"`
}

func decodeCalculatorArguments(raw json.RawMessage) (calculatorArguments, error) {
	var arguments calculatorArguments
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return calculatorArguments{}, ValidationErrorf("invalid JSON: %v", err)
	}
	if arguments.Left == nil {
		return calculatorArguments{}, ValidationErrorf("left is required")
	}
	if strings.TrimSpace(arguments.Operator) == "" {
		return calculatorArguments{}, ValidationErrorf("operator is required")
	}
	if arguments.Right == nil {
		return calculatorArguments{}, ValidationErrorf("right is required")
	}
	if !isCalculatorOperator(arguments.Operator) {
		return calculatorArguments{}, ValidationErrorf("unsupported operator %q", arguments.Operator)
	}

	return arguments, nil
}

func isCalculatorOperator(operator string) bool {
	switch operator {
	case "+", "-", "*", "/":
		return true
	default:
		return false
	}
}
