package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/zipkero/agent-runtime/internal/message"
)

// calculatorInput 은 calculator tool이 받는 입력 구조다.
// op: 연산자("add", "sub", "mul", "div"), a·b: 피연산자.
type calculatorInput struct {
	Op string  `json:"op"`
	A  float64 `json:"a"`
	B  float64 `json:"b"`
}

// calculatorInputSchema 는 LLM에 전달하는 입력 파라미터 JSON Schema다.
// 런타임 검증에는 쓰이지 않으며, LLM이 올바른 형태로 tool_call을 생성하도록 안내하는 용도다.
var calculatorInputSchema = json.RawMessage(`{
  "type": "object",
  "required": ["op", "a", "b"],
  "properties": {
    "op":  {"type": "string", "enum": ["add", "sub", "mul", "div"]},
    "a":   {"type": "number"},
    "b":   {"type": "number"}
  }
}`)

// Calculator 는 산술 연산을 수행하는 Tool 구현체다.
// 지원 연산: add(덧셈), sub(뺄셈), mul(곱셈), div(나눗셈).
// 잘못된 입력·미지원 연산·0 나눗셈은 error를 반환하며, Dispatcher가 IsError ToolResult로 정규화한다.
type Calculator struct{}

// Spec 은 tool 이름·설명·입력 schema를 반환한다.
func (c *Calculator) Spec() message.ToolSpec {
	return message.ToolSpec{
		Name:        "calculator",
		Description: "두 수에 대한 산술 연산(add/sub/mul/div)을 수행하고 결과를 반환한다.",
		InputSchema: calculatorInputSchema,
	}
}

// Execute 는 입력을 unmarshal·검증한 뒤 산술 결과를 ToolResult로 반환한다.
// 진입 직후 입력 검증을 먼저 수행하며, 실패 시 본체를 실행하지 않고 즉시 error를 반환한다.
func (c *Calculator) Execute(_ context.Context, input json.RawMessage) (message.ToolResult, error) {
	// 입력 unmarshal — JSON 형식 불량이면 즉시 error 반환 (본체 미실행)
	var in calculatorInput
	if err := json.Unmarshal(input, &in); err != nil {
		return message.ToolResult{}, fmt.Errorf("입력 파싱 실패: %w", err)
	}

	// 필드 검증 — op가 비어 있으면 malformed 입력으로 판단
	if in.Op == "" {
		return message.ToolResult{}, errors.New("입력 검증 실패: op 필드가 필요합니다")
	}

	// 연산 수행
	var result float64
	switch in.Op {
	case "add":
		result = in.A + in.B
	case "sub":
		result = in.A - in.B
	case "mul":
		result = in.A * in.B
	case "div":
		if in.B == 0 {
			return message.ToolResult{}, errors.New("0으로 나눌 수 없습니다")
		}
		result = in.A / in.B
	default:
		return message.ToolResult{}, fmt.Errorf("미지원 연산: %q (지원: add, sub, mul, div)", in.Op)
	}

	// 정수 표현 가능하면 정수로, 아니면 소수로 출력
	content := formatResult(result)
	return message.ToolResult{Content: content}, nil
}

// formatResult 는 float64 결과를 출력 문자열로 변환한다.
// 정수로 표현 가능한 경우 소수점 없이, 그 외에는 최대 10자리 소수로 출력한다.
func formatResult(v float64) string {
	if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}
