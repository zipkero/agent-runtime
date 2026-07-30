package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// TestCalculatorExecutesArithmetic 은 네 연산자의 계산 결과를 불필요한 소수점 없이 문자열 result로 정규화하는지 확인한다.
func TestCalculatorExecutesArithmetic(t *testing.T) {
	calculator := NewCalculator()
	tests := []struct {
		name string
		args json.RawMessage
		want string
	}{
		{name: "add", args: json.RawMessage(`{"left":2,"operator":"+","right":3}`), want: "5"},
		{name: "subtract", args: json.RawMessage(`{"left":7,"operator":"-","right":2}`), want: "5"},
		{name: "multiply", args: json.RawMessage(`{"left":2.5,"operator":"*","right":4}`), want: "10"},
		{name: "divide", args: json.RawMessage(`{"left":9,"operator":"/","right":2}`), want: "4.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := calculator.Validate(tt.args); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			got, err := calculator.Execute(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got.Content != tt.want {
				t.Fatalf("Execute() content = %q, want %q", got.Content, tt.want)
			}
		})
	}
}

// TestCalculatorRejectsInvalidArguments 는 JSON 오류, 피연산자 누락, 타입 불일치, 지원하지 않는 연산자를 실행 전 검증에서 거절하는지 확인한다.
func TestCalculatorRejectsInvalidArguments(t *testing.T) {
	calculator := NewCalculator()
	tests := []struct {
		name string
		args json.RawMessage
	}{
		{name: "invalid json", args: json.RawMessage(`{"left":2`)},
		{name: "missing left", args: json.RawMessage(`{"operator":"+","right":3}`)},
		{name: "missing operator", args: json.RawMessage(`{"left":2,"right":3}`)},
		{name: "missing right", args: json.RawMessage(`{"left":2,"operator":"+"}`)},
		{name: "unsupported operator", args: json.RawMessage(`{"left":2,"operator":"%","right":3}`)},
		{name: "wrong left type", args: json.RawMessage(`{"left":"2","operator":"+","right":3}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := calculator.Validate(tt.args)
			if !IsValidationError(err) {
				t.Fatalf("Validate() error = %v, want validation error", err)
			}
		})
	}
}

// TestCalculatorReturnsExecutionErrorForDivisionByZero 는 0으로 나누기가 입력 검증이 아니라 실행 오류로 분류되는지 확인한다.
func TestCalculatorReturnsExecutionErrorForDivisionByZero(t *testing.T) {
	calculator := NewCalculator()
	args := json.RawMessage(`{"left":9,"operator":"/","right":0}`)

	if err := calculator.Validate(args); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	_, err := calculator.Execute(context.Background(), args)
	if !IsExecutionError(err) {
		t.Fatalf("Execute() error = %v, want execution error", err)
	}
}

// TestCalculatorReturnsExecutionErrorWhenContextCanceled 은 취소된 ctx에서 계산을 진행하지 않고 취소 원인을 보존한 실행 오류를 반환하는지 확인한다.
func TestCalculatorReturnsExecutionErrorWhenContextCanceled(t *testing.T) {
	calculator := NewCalculator()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := calculator.Execute(ctx, json.RawMessage(`{"left":2,"operator":"+","right":3}`))
	if !IsExecutionError(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want canceled execution error", err)
	}
}

// TestCalculatorSchemaMatchesToolIdentity 는 Tool 이름과 schema의 이름·설명·InputSchema가 서로 어긋나지 않는지 확인한다.
func TestCalculatorSchemaMatchesToolIdentity(t *testing.T) {
	calculator := NewCalculator()
	schema := calculator.Schema()

	if calculator.Name() != "calculator" {
		t.Fatalf("Name() = %q, want calculator", calculator.Name())
	}
	if schema.Name != calculator.Name() {
		t.Fatalf("Schema().Name = %q, want %q", schema.Name, calculator.Name())
	}
	if schema.Description == "" || calculator.Description() == "" {
		t.Fatal("description must not be empty")
	}
	if !json.Valid(schema.InputSchema) {
		t.Fatalf("InputSchema is not valid JSON: %s", schema.InputSchema)
	}
}
