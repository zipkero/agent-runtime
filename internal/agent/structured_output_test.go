package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
)

// TestRunnerValidatesStructuredOutput 은 schema를 만족한 최종 답에서 앞뒤 공백만 제거한 원문을 StructuredOutput으로 돌려주고,
// FinalAnswer는 모델이 준 그대로 남기는지 확인한다.
func TestRunnerValidatesStructuredOutput(t *testing.T) {
	schema := json.RawMessage(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$defs": {
			"answer": {
				"type": "object",
				"required": ["name"],
				"properties": {"name": {"type": "string"}},
				"additionalProperties": false
			}
		},
		"$ref": "#/$defs/answer"
	}`)
	output := "{\n  \"name\": \"runtime\"\n}"
	assistantText := " \n" + output + "\t "
	client := &stubClient{response: llm.ChatResponse{Message: message.Assistant(assistantText)}}
	runner, err := NewRunner(RunnerOptions{
		Client:       client,
		MaxSteps:     1,
		OutputSchema: schema,
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result := runner.Run(context.Background(), "return JSON")

	if result.State.Status != StatusFinal || result.State.FinalAnswer != assistantText || result.State.LastError != nil {
		t.Fatalf("State = %+v, want validated final output", result.State)
	}
	if string(result.StructuredOutput) != output {
		t.Fatalf("StructuredOutput = %q, want original %q", result.StructuredOutput, output)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
}

// TestRunnerRejectsInvalidStructuredOutput 은 JSON 해석 실패와 schema 불일치를 실패한 단계까지 구분해 오류로 끊고,
// 원본 응답은 진단을 위해 남기는지 확인한다.
func TestRunnerRejectsInvalidStructuredOutput(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"required": ["name"],
		"properties": {"name": {"type": "string"}}
	}`)
	tests := []struct {
		name          string
		output        string
		wantOperation StructuredOutputOperation
	}{
		{name: "malformed JSON", output: `{"name":`, wantOperation: StructuredOutputOperationJSONParse},
		{name: "multiple JSON values", output: `{"name":"runtime"} {"name":"extra"}`, wantOperation: StructuredOutputOperationJSONParse},
		{name: "schema mismatch", output: `{"name": 42}`, wantOperation: StructuredOutputOperationValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &stubClient{response: llm.ChatResponse{Message: message.Assistant(tt.output)}}
			runner, err := NewRunner(RunnerOptions{Client: client, MaxSteps: 1, OutputSchema: schema})
			if err != nil {
				t.Fatalf("NewRunner() error = %v", err)
			}

			result := runner.Run(context.Background(), "return JSON")

			var runnerErr *RunnerError
			if result.State.Status != StatusError || result.State.FinalAnswer != "" ||
				!errors.As(result.State.LastError, &runnerErr) ||
				runnerErr.Kind != RunnerErrorKindStructuredOutput ||
				runnerErr.Operation != tt.wantOperation {
				t.Fatalf("State = %+v, want structured output %s error", result.State, tt.wantOperation)
			}
			if !IsRunnerErrorKind(result.State.LastError, RunnerErrorKindStructuredOutput) {
				t.Fatal("IsRunnerErrorKind() = false, want structured output")
			}
			if result.StructuredOutput != nil {
				t.Fatalf("StructuredOutput = %s, want nil", result.StructuredOutput)
			}
			if len(result.State.Messages) != 2 || result.State.Messages[1].Text != tt.output {
				t.Fatalf("Messages = %+v, want original assistant output", result.State.Messages)
			}
			lastTrace := result.State.Trace[len(result.State.Trace)-1]
			if lastTrace.Action != TraceActionStructuredOutputError || lastTrace.Status != StatusError ||
				!errors.Is(lastTrace.Error, result.State.LastError) {
				t.Fatalf("last trace = %+v, want structured output error", lastTrace)
			}
			if client.calls != 1 {
				t.Fatalf("client calls = %d, want 1", client.calls)
			}
		})
	}
}

// TestNewRunnerCompilesSelfContainedOutputSchema 는 빈 schema, 잘못된 schema, 외부 참조 schema를 provider 호출 전에 거절하는지 확인한다.
func TestNewRunnerCompilesSelfContainedOutputSchema(t *testing.T) {
	tests := []struct {
		name          string
		schema        json.RawMessage
		wantErrorText string
	}{
		{name: "empty schema", schema: json.RawMessage{}},
		{name: "malformed schema", schema: json.RawMessage(`{"type":`)},
		{name: "invalid schema", schema: json.RawMessage(`{"type": 42}`)},
		{
			name:          "external HTTP reference",
			schema:        json.RawMessage(`{"$ref":"https://example.com/schema.json"}`),
			wantErrorText: "external schema reference",
		},
		{
			name:          "external file reference",
			schema:        json.RawMessage(`{"$ref":"file:///tmp/schema.json"}`),
			wantErrorText: "external schema reference",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &stubClient{}
			_, err := NewRunner(RunnerOptions{Client: client, MaxSteps: 1, OutputSchema: tt.schema})
			var runnerErr *RunnerError
			if !errors.As(err, &runnerErr) ||
				runnerErr.Kind != RunnerErrorKindStructuredOutput ||
				runnerErr.Operation != StructuredOutputOperationSchemaCompile {
				t.Fatalf("NewRunner() error = %v, want structured output schema compile error", err)
			}
			if !IsRunnerErrorKind(err, RunnerErrorKindStructuredOutput) {
				t.Fatal("IsRunnerErrorKind() = false, want structured output")
			}
			if tt.wantErrorText != "" && !strings.Contains(err.Error(), tt.wantErrorText) {
				t.Fatalf("NewRunner() error = %q, want %q context", err, tt.wantErrorText)
			}
			if client.calls != 0 {
				t.Fatalf("client calls = %d, want 0 before schema compile succeeds", client.calls)
			}
		})
	}
}
