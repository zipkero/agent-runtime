package tool_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// calcInput 은 테스트용 calculator 입력 JSON을 만드는 헬퍼다.
func calcInput(op string, a, b float64) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"op": op, "a": a, "b": b})
	return raw
}

func TestCalculator_Spec(t *testing.T) {
	c := &tool.Calculator{}
	spec := c.Spec()

	if spec.Name != "calculator" {
		t.Errorf("Spec().Name = %q, want %q", spec.Name, "calculator")
	}
	if spec.Description == "" {
		t.Error("Spec().Description이 비어 있습니다")
	}
	if len(spec.InputSchema) == 0 {
		t.Error("Spec().InputSchema가 비어 있습니다")
	}
}

// TestCalculator_Operations 는 지원 연산 4종이 기대 결과를 반환하는지 검증한다.
func TestCalculator_Operations(t *testing.T) {
	c := &tool.Calculator{}
	ctx := context.Background()

	tests := []struct {
		name string
		op   string
		a, b float64
		want string
	}{
		{"덧셈 정수", "add", 3, 4, "7"},
		{"뺄셈 정수", "sub", 10, 3, "7"},
		{"곱셈 정수", "mul", 6, 7, "42"},
		{"나눗셈 나누어 떨어짐", "div", 10, 2, "5"},
		{"나눗셈 소수", "div", 1, 3, "0.3333333333333333"},
		{"덧셈 음수", "add", -5, 3, "-2"},
		{"곱셈 소수", "mul", 2.5, 4, "10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := c.Execute(ctx, calcInput(tt.op, tt.a, tt.b))
			if err != nil {
				t.Fatalf("예상치 못한 error: %v", err)
			}
			if result.IsError {
				t.Fatalf("IsError=true, Content=%q", result.Content)
			}
			if result.Content != tt.want {
				t.Errorf("Content = %q, want %q", result.Content, tt.want)
			}
		})
	}
}

// TestCalculator_DivisionByZero 는 0 나눗셈이 error를 반환하는지 검증한다.
func TestCalculator_DivisionByZero(t *testing.T) {
	c := &tool.Calculator{}
	_, err := c.Execute(context.Background(), calcInput("div", 5, 0))
	if err == nil {
		t.Fatal("0 나눗셈에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

// TestCalculator_UnsupportedOp 는 미지원 연산자가 error를 반환하는지 검증한다.
func TestCalculator_UnsupportedOp(t *testing.T) {
	c := &tool.Calculator{}
	_, err := c.Execute(context.Background(), calcInput("mod", 10, 3))
	if err == nil {
		t.Fatal("미지원 연산에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

// TestCalculator_MalformedJSON 은 JSON 파싱 불가 입력이 error를 반환하는지 검증한다.
func TestCalculator_MalformedJSON(t *testing.T) {
	c := &tool.Calculator{}
	_, err := c.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("malformed JSON에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

// TestCalculator_MissingOp 는 op 필드가 없는 입력이 error를 반환하는지 검증한다.
func TestCalculator_MissingOp(t *testing.T) {
	c := &tool.Calculator{}
	raw, _ := json.Marshal(map[string]any{"a": 1, "b": 2}) // op 없음
	_, err := c.Execute(context.Background(), raw)
	if err == nil {
		t.Fatal("op 필드 누락에서 error를 반환해야 하는데 nil을 반환했습니다")
	}
}

// TestCalculator_ViaDispatcher 는 Dispatcher를 통해 calculator가 올바르게 동작하는지 검증한다.
// Dispatcher의 IsError 정규화 경로를 calculator와 함께 확인한다.
func TestCalculator_ViaDispatcher(t *testing.T) {
	d := newDispatcher(time.Second, &tool.Calculator{})

	t.Run("성공 경로", func(t *testing.T) {
		call := message.ToolCall{ID: "calc-1", Name: "calculator", Input: calcInput("mul", 3, 7)}
		result := d.Dispatch(context.Background(), call)

		if result.IsError {
			t.Fatalf("IsError=true, Content=%q", result.Content)
		}
		if result.ToolCallID != call.ID {
			t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, call.ID)
		}
		if result.Content != "21" {
			t.Errorf("Content = %q, want %q", result.Content, "21")
		}
	})

	t.Run("0 나눗셈은 IsError로 정규화", func(t *testing.T) {
		call := message.ToolCall{ID: "calc-2", Name: "calculator", Input: calcInput("div", 1, 0)}
		result := d.Dispatch(context.Background(), call)

		if !result.IsError {
			t.Fatal("0 나눗셈 결과가 IsError=true로 정규화되어야 하는데 false를 반환했습니다")
		}
		if result.ToolCallID != call.ID {
			t.Errorf("ToolCallID = %q, want %q", result.ToolCallID, call.ID)
		}
	})

	t.Run("미지원 연산은 IsError로 정규화", func(t *testing.T) {
		call := message.ToolCall{ID: "calc-3", Name: "calculator", Input: calcInput("pow", 2, 8)}
		result := d.Dispatch(context.Background(), call)

		if !result.IsError {
			t.Fatal("미지원 연산 결과가 IsError=true로 정규화되어야 하는데 false를 반환했습니다")
		}
	})
}
