package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

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

func TestCalculatorReturnsExecutionErrorWhenContextCanceled(t *testing.T) {
	calculator := NewCalculator()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := calculator.Execute(ctx, json.RawMessage(`{"left":2,"operator":"+","right":3}`))
	if !IsExecutionError(err) || !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want canceled execution error", err)
	}
}

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
