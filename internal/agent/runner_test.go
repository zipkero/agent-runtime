package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/agent"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// TestRunner_NoOutputContractUsesTextOnlyResult 는 output contract가 없을 때 기존 text-only 결과 표면이
// structured output 필드 없이 그대로 유지되어야 한다는 회귀 조건을 검증한다.
func TestRunner_NoOutputContractUsesTextOnlyResult(t *testing.T) {
	const promptText = "runner text-only 프롬프트"
	const replyText = "runner 최종 답"

	stub := &seqStub{
		responses: []llm.ChatResponse{textResponse(replyText)},
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:   stub,
		Model:    "stub-model",
		MaxSteps: 5,
		Registry: tool.NewRegistry(),
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), promptText)

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if result.State.Status != agent.StatusFinal {
		t.Errorf("AgentState.Status 기대값 final, 실제값 %q", result.State.Status)
	}
	if result.FinalMessage.Role != message.RoleAssistant {
		t.Errorf("FinalMessage.Role 기대값 assistant, 실제값 %q", result.FinalMessage.Role)
	}
	if result.FinalText != replyText {
		t.Errorf("FinalText 기대값 %q, 실제값 %q", replyText, result.FinalText)
	}
	if result.Err != nil {
		t.Errorf("success 결과에서 Err는 nil이어야 하나 Err=%v", result.Err)
	}
	if result.StructuredRaw != nil {
		t.Errorf("output contract 없는 결과의 StructuredRaw는 nil이어야 하나 실제값 %s", result.StructuredRaw)
	}
	if result.StructuredValue != nil {
		t.Errorf("output contract 없는 결과의 StructuredValue는 nil이어야 하나 실제값 %#v", result.StructuredValue)
	}
}

// TestRunner_OutputContractAddsSystemInstruction 는 output contract가 있으면 Runner가 실제 LLM 요청 앞쪽에
// structured output system message를 추가해야 한다는 계약을 검증한다.
func TestRunner_OutputContractAddsSystemInstruction(t *testing.T) {
	const finalText = `{"answer":"완료"}`
	capturer := &capturingStub{
		seqStub: seqStub{responses: []llm.ChatResponse{textResponse(finalText)}},
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:   capturer,
		Model:    "stub-model",
		MaxSteps: 5,
		Registry: tool.NewRegistry(),
		Output: &agent.OutputContract{
			Name:        "answer_result",
			Description: "테스트용 구조화 응답",
			Schema:      json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "structured output 요청 지시 테스트")

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if len(capturer.capturedRequests) != 1 {
		t.Fatalf("ChatRequest 캡처 수 기대값 1, 실제값 %d", len(capturer.capturedRequests))
	}
	assertStructuredOutputInstruction(t, capturer.capturedRequests[0], "answer_result")
}

// TestRunner_OutputContractInstructionRunsBeforeUserMiddleware 는 built-in PreModel 단계가 사용자 middleware보다
// 먼저 실행되어 사용자 middleware도 output contract 지시가 포함된 요청을 관찰해야 한다는 계약을 검증한다.
func TestRunner_OutputContractInstructionRunsBeforeUserMiddleware(t *testing.T) {
	const finalText = `{"answer":"완료"}`
	observed := false
	userMiddleware := middlewareStub{
		pre: func(_ context.Context, in agent.PreModelInput) (llm.ChatRequest, error) {
			observed = true
			assertStructuredOutputInstruction(t, in.Request, "answer_result")
			return in.Request, nil
		},
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:     &seqStub{responses: []llm.ChatResponse{textResponse(finalText)}},
		Model:      "stub-model",
		MaxSteps:   5,
		Registry:   tool.NewRegistry(),
		Middleware: []agent.Middleware{userMiddleware},
		Output: &agent.OutputContract{
			Name:   "answer_result",
			Schema: json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "structured output middleware 순서 테스트")

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if !observed {
		t.Fatal("사용자 middleware가 output contract 지시가 포함된 요청을 관찰하지 못했다")
	}
}

// TestRunner_NoOutputContractDoesNotAddSystemInstruction 는 output contract가 없으면 Runner가 기존 text-only
// 요청 메시지 구성을 변경하지 않아야 한다는 회귀 조건을 검증한다.
func TestRunner_NoOutputContractDoesNotAddSystemInstruction(t *testing.T) {
	capturer := &capturingStub{
		seqStub: seqStub{responses: []llm.ChatResponse{textResponse("최종 답")}},
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:   capturer,
		Model:    "stub-model",
		MaxSteps: 5,
		Registry: tool.NewRegistry(),
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "contract 미지정 요청 테스트")

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if len(capturer.capturedRequests) != 1 {
		t.Fatalf("ChatRequest 캡처 수 기대값 1, 실제값 %d", len(capturer.capturedRequests))
	}
	req := capturer.capturedRequests[0]
	if len(req.Messages) != 1 {
		t.Fatalf("contract 미지정 요청 메시지 수 기대값 1, 실제값 %d", len(req.Messages))
	}
	if req.Messages[0].Role != message.RoleUser {
		t.Errorf("contract 미지정 요청 첫 메시지 Role 기대값 user, 실제값 %q", req.Messages[0].Role)
	}
}

// TestRunner_OutputContractInstructionPersistsAfterToolCall 는 tool call 이후 재요청되는 LLM 요청에도
// structured output system message가 계속 포함되어야 한다는 계약을 검증한다.
func TestRunner_OutputContractInstructionPersistsAfterToolCall(t *testing.T) {
	const finalText = `{"answer":"도구 완료"}`
	ft := &fakeTool{name: "lookup", result: "검색 결과"}
	reg := tool.NewRegistry()
	if err := reg.Register(ft); err != nil {
		t.Fatalf("tool 등록 실패: %v", err)
	}
	capturer := &capturingStub{
		seqStub: seqStub{responses: []llm.ChatResponse{toolCallResponse(), textResponse(finalText)}},
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:   capturer,
		Model:    "stub-model",
		MaxSteps: 5,
		Registry: reg,
		Output: &agent.OutputContract{
			Name:   "answer_result",
			Schema: json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "structured output tool call 테스트")

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if len(capturer.capturedRequests) != 2 {
		t.Fatalf("ChatRequest 캡처 수 기대값 2, 실제값 %d", len(capturer.capturedRequests))
	}
	assertStructuredOutputInstruction(t, capturer.capturedRequests[0], "answer_result")
	assertStructuredOutputInstruction(t, capturer.capturedRequests[1], "answer_result")
	if ft.calls != 1 {
		t.Errorf("fakeTool.Execute 호출 횟수 기대값 1, 실제값 %d", ft.calls)
	}
}

// TestRunner_StructuredOutputSuccess 는 최종 assistant JSON text가 output contract를 만족하면 raw JSON과
// decoded value가 success 결과에 함께 포함되어야 한다는 계약을 검증한다.
func TestRunner_StructuredOutputSuccess(t *testing.T) {
	const finalText = `{"answer":"완료","score":0.9}`
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:   &seqStub{responses: []llm.ChatResponse{textResponse(finalText)}},
		Model:    "stub-model",
		MaxSteps: 5,
		Registry: tool.NewRegistry(),
		Output: &agent.OutputContract{
			Name: "answer_result",
			Schema: json.RawMessage(`{
				"type": "object",
				"required": ["answer", "score"],
				"properties": {
					"answer": {"type": "string"},
					"score": {"type": "number"}
				},
				"additionalProperties": false
			}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "structured output 성공 테스트")

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if result.FinalText != finalText {
		t.Errorf("FinalText 기대값 %q, 실제값 %q", finalText, result.FinalText)
	}
	if string(result.StructuredRaw) != finalText {
		t.Errorf("StructuredRaw 기대값 %q, 실제값 %q", finalText, result.StructuredRaw)
	}
	value, ok := result.StructuredValue.(map[string]any)
	if !ok {
		t.Fatalf("StructuredValue 타입 기대값 map[string]any, 실제값 %#v", result.StructuredValue)
	}
	if value["answer"] != "완료" {
		t.Errorf("StructuredValue[answer] 기대값 %q, 실제값 %#v", "완료", value["answer"])
	}
	if value["score"] != 0.9 {
		t.Errorf("StructuredValue[score] 기대값 0.9, 실제값 %#v", value["score"])
	}
}

// TestRunner_StructuredOutputMalformedJSONFails 는 최종 text가 JSON으로 파싱되지 않으면 Agent state와 raw text를
// 보존하면서 structured output parse 실패로 분류해야 한다는 계약을 검증한다.
func TestRunner_StructuredOutputMalformedJSONFails(t *testing.T) {
	const finalText = `not-json`
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:   &seqStub{responses: []llm.ChatResponse{textResponse(finalText)}},
		Model:    "stub-model",
		MaxSteps: 5,
		Registry: tool.NewRegistry(),
		Output: &agent.OutputContract{
			Name:   "answer_result",
			Schema: json.RawMessage(`{"type":"object"}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "structured output JSON 파싱 실패 테스트")

	assertStructuredOutputError(t, result, finalText, agent.StructuredOutputErrorParse)
}

// TestRunner_StructuredOutputSchemaValidationFails 는 JSON 파싱은 성공했지만 schema를 만족하지 않는 최종 text를
// structured output validation 실패로 분류해야 한다는 계약을 검증한다.
func TestRunner_StructuredOutputSchemaValidationFails(t *testing.T) {
	const finalText = `{"answer":7}`
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:   &seqStub{responses: []llm.ChatResponse{textResponse(finalText)}},
		Model:    "stub-model",
		MaxSteps: 5,
		Registry: tool.NewRegistry(),
		Output: &agent.OutputContract{
			Name: "answer_result",
			Schema: json.RawMessage(`{
				"type": "object",
				"required": ["answer"],
				"properties": {
					"answer": {"type": "string"}
				}
			}`),
		},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "structured output schema 검증 실패 테스트")

	assertStructuredOutputError(t, result, finalText, agent.StructuredOutputErrorValidation)
}

// TestRunner_StructuredOutputEmptySchemaFails 는 비어 있는 output schema가 조용한 fallback 없이
// structured output invalid_schema 실패로 분류되어야 한다는 계약을 검증한다.
func TestRunner_StructuredOutputEmptySchemaFails(t *testing.T) {
	const finalText = `{"answer":"완료"}`
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:   &seqStub{responses: []llm.ChatResponse{textResponse(finalText)}},
		Model:    "stub-model",
		MaxSteps: 5,
		Registry: tool.NewRegistry(),
		Output: &agent.OutputContract{
			Name:   "answer_result",
			Schema: json.RawMessage(`  `),
		},
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "structured output 빈 schema 테스트")

	assertStructuredOutputError(t, result, finalText, agent.StructuredOutputErrorInvalidSchema)
}

func TestRunner_ToolCallExecutesAndAccumulates(t *testing.T) {
	const toolName = "lookup"
	const toolResult = "runner 검색 결과"
	const finalText = "runner tool 최종 답"

	ft := &fakeTool{name: toolName, result: toolResult}
	reg := tool.NewRegistry()
	if err := reg.Register(ft); err != nil {
		t.Fatalf("tool 등록 실패: %v", err)
	}

	stub := &seqStub{
		responses: []llm.ChatResponse{
			toolCallResponse(),
			textResponse(finalText),
		},
	}
	runner, err := agent.NewRunner(agent.RunnerConfig{
		Client:      stub,
		Model:       "stub-model",
		MaxSteps:    5,
		Registry:    reg,
		ToolTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewRunner 실패: %v", err)
	}

	result := runner.Run(context.Background(), "runner tool 실행 테스트")

	if result.Status != agent.RunnerStatusSuccess {
		t.Fatalf("RunnerStatus 기대값 success, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if result.FinalText != finalText {
		t.Errorf("FinalText 기대값 %q, 실제값 %q", finalText, result.FinalText)
	}
	if ft.calls != 1 {
		t.Errorf("fakeTool.Execute 호출 횟수 기대값 1, 실제값 %d", ft.calls)
	}
	if stub.calls != 2 {
		t.Errorf("LLM 호출 횟수 기대값 2, 실제값 %d", stub.calls)
	}
	if len(result.State.Messages) != 4 {
		t.Fatalf("누적 메시지 수 기대값 4, 실제값 %d", len(result.State.Messages))
	}
	toolMsg := result.State.Messages[2]
	if toolMsg.Role != message.RoleTool {
		t.Fatalf("Messages[2].Role 기대값 tool, 실제값 %q", toolMsg.Role)
	}
	if len(toolMsg.Content) == 0 || toolMsg.Content[0].ToolResult == nil {
		t.Fatal("RoleTool 메시지에 ToolResult가 없다")
	}
	if toolMsg.Content[0].ToolResult.Content != toolResult {
		t.Errorf("ToolResult.Content 기대값 %q, 실제값 %q", toolResult, toolMsg.Content[0].ToolResult.Content)
	}
}

func TestRunner_StatusMapping_MaxStepsAndError(t *testing.T) {
	t.Run("max steps", func(t *testing.T) {
		stub := &seqStub{
			responses: []llm.ChatResponse{toolCallResponse()},
		}
		runner, err := agent.NewRunner(agent.RunnerConfig{
			Client:   stub,
			Model:    "stub-model",
			MaxSteps: 2,
			Registry: tool.NewRegistry(),
		})
		if err != nil {
			t.Fatalf("NewRunner 실패: %v", err)
		}

		result := runner.Run(context.Background(), "runner max steps 테스트")

		if result.Status != agent.RunnerStatusMaxSteps {
			t.Fatalf("RunnerStatus 기대값 max_steps, 실제값 %q", result.Status)
		}
		if result.State.Status != agent.StatusMaxSteps {
			t.Errorf("AgentState.Status 기대값 max_steps, 실제값 %q", result.State.Status)
		}
		if result.FinalText != "" {
			t.Errorf("max_steps 결과에서 FinalText는 비어야 하나 실제값 %q", result.FinalText)
		}
		if result.Err != nil {
			t.Errorf("max_steps 결과에서 Err는 nil이어야 하나 Err=%v", result.Err)
		}
	})

	t.Run("agent error", func(t *testing.T) {
		sentinelErr := errors.New("runner LLM 호출 실패")
		stub := &seqStub{err: sentinelErr}
		runner, err := agent.NewRunner(agent.RunnerConfig{
			Client:   stub,
			Model:    "stub-model",
			MaxSteps: 5,
			Registry: tool.NewRegistry(),
		})
		if err != nil {
			t.Fatalf("NewRunner 실패: %v", err)
		}

		result := runner.Run(context.Background(), "runner error 테스트")

		if result.Status != agent.RunnerStatusAgentError {
			t.Fatalf("RunnerStatus 기대값 agent_error, 실제값 %q", result.Status)
		}
		if result.State.Status != agent.StatusError {
			t.Errorf("AgentState.Status 기대값 error, 실제값 %q", result.State.Status)
		}
		if !errors.Is(result.Err, sentinelErr) {
			t.Errorf("errors.Is 실패: result.Err=%v, 기대값 %v", result.Err, sentinelErr)
		}
		if result.FinalText != "" {
			t.Errorf("agent_error 결과에서 FinalText는 비어야 하나 실제값 %q", result.FinalText)
		}
	})
}

func assertStructuredOutputError(t *testing.T, result agent.RunnerResult, finalText string, wantKind agent.StructuredOutputErrorKind) {
	t.Helper()
	if result.Status != agent.RunnerStatusStructuredOutputError {
		t.Fatalf("RunnerStatus 기대값 structured_output_error, 실제값 %q, err=%v", result.Status, result.Err)
	}
	if result.State.Status != agent.StatusFinal {
		t.Errorf("AgentState.Status 기대값 final, 실제값 %q", result.State.Status)
	}
	if result.FinalText != finalText {
		t.Errorf("FinalText 기대값 %q, 실제값 %q", finalText, result.FinalText)
	}
	if result.FinalMessage.Role != message.RoleAssistant {
		t.Errorf("FinalMessage.Role 기대값 assistant, 실제값 %q", result.FinalMessage.Role)
	}
	if result.StructuredRaw != nil {
		t.Errorf("실패 결과의 StructuredRaw는 nil이어야 하나 실제값 %s", result.StructuredRaw)
	}
	if result.StructuredValue != nil {
		t.Errorf("실패 결과의 StructuredValue는 nil이어야 하나 실제값 %#v", result.StructuredValue)
	}
	var structuredErr *agent.StructuredOutputError
	if !errors.As(result.Err, &structuredErr) {
		t.Fatalf("result.Err는 StructuredOutputError로 확인되어야 한다: %v", result.Err)
	}
	if structuredErr.Kind != wantKind {
		t.Errorf("StructuredOutputError.Kind 기대값 %q, 실제값 %q", wantKind, structuredErr.Kind)
	}
}

func assertStructuredOutputInstruction(t *testing.T, req llm.ChatRequest, contractName string) {
	t.Helper()
	if len(req.Messages) == 0 {
		t.Fatal("ChatRequest.Messages가 비어 있다")
	}
	systemMsg := req.Messages[0]
	if systemMsg.Role != message.RoleSystem {
		t.Fatalf("structured output 지시는 첫 system message여야 한다. 실제 Role=%q", systemMsg.Role)
	}
	if len(systemMsg.Content) == 0 || systemMsg.Content[0].Type != message.BlockTypeText {
		t.Fatalf("structured output system message에 text block이 없다: %#v", systemMsg.Content)
	}
	text := systemMsg.Content[0].Text
	for _, want := range []string{
		"final assistant response as JSON only",
		contractName,
		"JSON Schema:",
		`"answer":{"type":"string"}`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("structured output system message에 %q가 없다: %q", want, text)
		}
	}
}
