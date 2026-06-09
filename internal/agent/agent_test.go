package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/zipkero/agent-runtime/internal/agent"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// seqStub 은 step마다 다른 응답을 순서대로 반환하는 다단계 LLMClient 구현체다.
// stub_test.go의 StubClient와 동일하게 ctx 취소를 먼저 존중하고 호출 횟수를 기록한다.
type seqStub struct {
	responses []llm.ChatResponse
	err       error      // err != nil이면 모든 호출에서 이 에러를 반환한다
	calls     int        // 실제로 응답을 반환한 호출 횟수
}

// Chat 은 ctx 취소를 먼저 확인하고, 순서대로 응답을 반환한다.
// 시퀀스를 소진하면 마지막 응답을 반복 반환해 step을 계속 진행하게 한다.
func (s *seqStub) Chat(ctx context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	// 실제 client처럼 취소된 ctx를 먼저 존중한다
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	if s.err != nil {
		s.calls++
		return llm.ChatResponse{}, s.err
	}
	idx := s.calls
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	s.calls++
	return s.responses[idx], nil
}

// 컴파일 타임에 llm.LLMClient 인터페이스를 구현하는지 확인한다.
var _ llm.LLMClient = (*seqStub)(nil)

// toolCallResponse 는 tool_call ContentBlock을 포함한 assistant 응답을 생성하는 헬퍼다.
func toolCallResponse() llm.ChatResponse {
	return llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_001",
					Name:  "lookup",
					Input: json.RawMessage(`{"query":"test"}`),
				}),
			},
		},
	}
}

// textResponse 는 text ContentBlock만 포함한 assistant 응답을 생성하는 헬퍼다.
func textResponse(text string) llm.ChatResponse {
	return llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewTextBlock(text),
			},
		},
	}
}

// TestRun_NormalExit 는 첫 응답이 tool_call 없는 text일 때 즉시 StatusFinal로 종료되는지 검증한다.
// 확인 항목:
//   - 종료 상태가 StatusFinal
//   - FinalMessage가 응답 text를 반환하고 ok=true
//   - 누적 메시지에 user 입력과 assistant 응답이 모두 포함
//   - Steps가 1 증가(1 step 진행)
func TestRun_NormalExit(t *testing.T) {
	const promptText = "안녕하세요"
	const replyText = "안녕하세요! 무엇을 도와드릴까요?"

	stub := &seqStub{
		responses: []llm.ChatResponse{textResponse(replyText)},
	}
	a := agent.NewAgent(stub, "stub-model", 5, nil, tool.NewRegistry(), 0)

	state := a.Run(context.Background(), promptText)

	// 종료 상태 검증
	if state.Status != agent.StatusFinal {
		t.Errorf("종료 상태 기대값 StatusFinal, 실제값 %q", state.Status)
	}

	// FinalMessage 검증
	finalMsg, ok := state.FinalMessage()
	if !ok {
		t.Fatal("FinalMessage가 ok=false를 반환했으나 StatusFinal 상태에서 true를 기대했다")
	}
	if finalMsg.Role != message.RoleAssistant {
		t.Errorf("FinalMessage.Role 기대값 assistant, 실제값 %q", finalMsg.Role)
	}
	if len(finalMsg.Content) == 0 || finalMsg.Content[0].Text != replyText {
		t.Errorf("FinalMessage 텍스트 기대값 %q, 실제값 %q", replyText, finalMsg.Content[0].Text)
	}

	// 누적 메시지 검증: user 입력 + assistant 응답
	if len(state.Messages) != 2 {
		t.Fatalf("누적 메시지 수 기대값 2, 실제값 %d", len(state.Messages))
	}
	if state.Messages[0].Role != message.RoleUser {
		t.Errorf("첫 메시지 role 기대값 user, 실제값 %q", state.Messages[0].Role)
	}
	if state.Messages[1].Role != message.RoleAssistant {
		t.Errorf("두 번째 메시지 role 기대값 assistant, 실제값 %q", state.Messages[1].Role)
	}

	// step 증가 검증
	if state.Steps != 1 {
		t.Errorf("Steps 기대값 1, 실제값 %d", state.Steps)
	}

	// 에러 없음 검증
	if state.Err != nil {
		t.Errorf("에러 없음을 기대했으나 Err=%v", state.Err)
	}
}

// TestRun_MaxSteps 는 매 응답이 tool_call일 때 maxSteps 소진 후 StatusMaxSteps로 종료되는지 검증한다.
// 핵심 단언: LLM 호출 횟수가 maxSteps와 정확히 일치한다(선검사로 초과 호출 없음).
func TestRun_MaxSteps(t *testing.T) {
	const maxSteps = 3

	stub := &seqStub{
		responses: []llm.ChatResponse{toolCallResponse()},
	}
	a := agent.NewAgent(stub, "stub-model", maxSteps, nil, tool.NewRegistry(), 0)

	state := a.Run(context.Background(), "반복 tool_call 프롬프트")

	// 종료 상태 검증
	if state.Status != agent.StatusMaxSteps {
		t.Errorf("종료 상태 기대값 StatusMaxSteps, 실제값 %q", state.Status)
	}

	// final이 아님을 명시적으로 검증
	if state.Status == agent.StatusFinal {
		t.Error("StatusMaxSteps 상태에서 StatusFinal이 아니어야 한다")
	}
	_, ok := state.FinalMessage()
	if ok {
		t.Error("StatusMaxSteps 상태에서 FinalMessage가 ok=true를 반환해서는 안 된다")
	}

	// LLM 호출 횟수가 maxSteps와 정확히 일치하는지 검증(핵심 단언)
	if stub.calls != maxSteps {
		t.Errorf("LLM 호출 횟수 기대값 %d, 실제값 %d — 선검사로 초과 호출이 없어야 한다",
			maxSteps, stub.calls)
	}

	// step 수가 maxSteps와 일치하는지 검증
	if state.Steps != maxSteps {
		t.Errorf("Steps 기대값 %d, 실제값 %d", maxSteps, state.Steps)
	}

	// 에러 없음 검증(max step은 error 상태가 아님)
	if state.Err != nil {
		t.Errorf("StatusMaxSteps에서 Err는 nil이어야 하나 Err=%v", state.Err)
	}
}

// TestRun_ErrorExit 는 stub이 에러를 반환할 때 StatusError로 종료되고 원인이 state에 보관되는지 검증한다.
func TestRun_ErrorExit(t *testing.T) {
	sentinelErr := errors.New("LLM 호출 실패")

	stub := &seqStub{err: sentinelErr}
	a := agent.NewAgent(stub, "stub-model", 5, nil, tool.NewRegistry(), 0)

	state := a.Run(context.Background(), "에러 케이스 프롬프트")

	// 종료 상태 검증
	if state.Status != agent.StatusError {
		t.Errorf("종료 상태 기대값 StatusError, 실제값 %q", state.Status)
	}

	// 원인 에러 검증
	if !errors.Is(state.Err, sentinelErr) {
		t.Errorf("errors.Is 실패: state.Err=%v, 기대값 %v", state.Err, sentinelErr)
	}

	// final이 아님 검증
	_, ok := state.FinalMessage()
	if ok {
		t.Error("StatusError 상태에서 FinalMessage가 ok=true를 반환해서는 안 된다")
	}
}

// TestRun_HookObservation 은 ReflectionHook이 step 경계마다 호출되며
// 캡처한 step 번호와 state로 호출 사실을 확인한다.
func TestRun_HookObservation(t *testing.T) {
	// 첫 두 번은 tool_call, 세 번째는 text → 2 step 후 StatusFinal
	stub := &seqStub{
		responses: []llm.ChatResponse{
			toolCallResponse(),
			toolCallResponse(),
			textResponse("최종 답"),
		},
	}

	type hookRecord struct {
		step  int
		steps int // 호출 시점의 state.Steps
	}
	var records []hookRecord

	hook := func(step int, state agent.AgentState) {
		records = append(records, hookRecord{step: step, steps: state.Steps})
	}

	a := agent.NewAgent(stub, "stub-model", 5, hook, tool.NewRegistry(), 0)
	state := a.Run(context.Background(), "hook 관찰 프롬프트")

	// 정상 종료 확인
	if state.Status != agent.StatusFinal {
		t.Errorf("종료 상태 기대값 StatusFinal, 실제값 %q", state.Status)
	}

	// hook이 3번 호출돼야 한다(step 0, 1, 2 진입 시)
	expectedCalls := 3
	if len(records) != expectedCalls {
		t.Fatalf("hook 호출 횟수 기대값 %d, 실제값 %d", expectedCalls, len(records))
	}

	// 각 호출 시점의 step 번호 검증
	for i, rec := range records {
		if rec.step != i {
			t.Errorf("records[%d]: step 기대값 %d, 실제값 %d", i, i, rec.step)
		}
		// hook 호출 시점의 state.Steps는 아직 해당 step을 진행하기 전이므로 i와 같다
		if rec.steps != i {
			t.Errorf("records[%d]: state.Steps 기대값 %d, 실제값 %d", i, i, rec.steps)
		}
	}
}
