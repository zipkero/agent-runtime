package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/agent"
	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// capturingStub 은 seqStub과 동일하게 순서대로 응답하되,
// 각 Chat 호출 시 전달된 ChatRequest를 순서대로 캡처한다.
type capturingStub struct {
	seqStub
	capturedRequests []llm.ChatRequest
}

func (s *capturingStub) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	s.capturedRequests = append(s.capturedRequests, req)
	return s.seqStub.Chat(ctx, req)
}

// fakeTool 은 테스트용 Tool 구현체다. Execute가 호출되면 calls를 증가시키고 지정한 결과를 반환한다.
type fakeTool struct {
	name   string
	result string
	calls  int
}

func (f *fakeTool) Spec() message.ToolSpec {
	return message.ToolSpec{
		Name:        f.name,
		Description: f.name + " 테스트용 tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (message.ToolResult, error) {
	f.calls++
	return message.ToolResult{Content: f.result}, nil
}

// seqStub 은 step마다 다른 응답을 순서대로 반환하는 다단계 LLMClient 구현체다.
// stub_test.go의 StubClient와 동일하게 ctx 취소를 먼저 존중하고 호출 횟수를 기록한다.
type seqStub struct {
	responses []llm.ChatResponse
	err       error // err != nil이면 모든 호출에서 이 에러를 반환한다
	calls     int   // 실제로 응답을 반환한 호출 횟수
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

func phase51ToolBundleResponse() llm.ChatResponse {
	return llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_web",
					Name:  "web_search",
					Input: json.RawMessage(`{"query":"agent runtime"}`),
				}),
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_save",
					Name:  "file_save",
					Input: json.RawMessage(`{"path":"agent-output.txt","content":"saved by agent regression"}`),
				}),
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_code",
					Name:  "code_execute",
					Input: json.RawMessage(`{"profile":"helper","args":["stdout"]}`),
				}),
			},
		},
	}
}

func newPhase51TestRegistry(t *testing.T, workDir string) *tool.Registry {
	t.Helper()

	reg := tool.NewRegistry()
	if err := reg.Register(tool.NewWebSearch("", nil)); err != nil {
		t.Fatalf("web_search 등록 실패: %v", err)
	}
	fs, err := tool.NewFileSave(workDir)
	if err != nil {
		t.Fatalf("file_save 생성 실패: %v", err)
	}
	if err := reg.Register(fs); err != nil {
		t.Fatalf("file_save 등록 실패: %v", err)
	}
	ce, err := tool.NewCodeExecution(workDir, []tool.CommandProfile{
		{
			Name:        "helper",
			Command:     os.Args[0],
			Args:        []string{"-test.run=TestRun_Phase51ToolBundle_HelperProcess", "--"},
			Env:         []string{"GO_WANT_AGENT_PHASE51_HELPER=1"},
			AllowedArgs: []string{"stdout"},
		},
	}, 1024)
	if err != nil {
		t.Fatalf("code_execute 생성 실패: %v", err)
	}
	if err := reg.Register(ce); err != nil {
		t.Fatalf("code_execute 등록 실패: %v", err)
	}
	return reg
}

func TestRun_Phase51ToolBundle_HelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_AGENT_PHASE51_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 || args[1] != "stdout" {
		fmt.Fprint(os.Stderr, "unexpected helper args")
		os.Exit(2)
	}
	fmt.Print("agent code ok")
	os.Exit(0)
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

// TestRun_ToolCall_ExecutedAndAccumulated 는 tool이 등록된 registry로 tool_call 한 회전 뒤
// RoleTool 메시지가 누적되고, 이후 text 응답에서 StatusFinal로 종료되는지 검증한다.
// 확인 항목:
//   - tool.Execute가 실제 호출됨
//   - tool_call 회전 뒤 RoleTool 메시지가 state.Messages에 추가됨
//   - RoleTool 메시지의 ContentBlock에 ToolResult가 담겨 있음
//   - 이후 text 응답에서 StatusFinal로 종료됨
//   - LLM 호출 횟수 = 2 (tool_call 회전 1 + text 회전 1)
func TestRun_ToolCall_ExecutedAndAccumulated(t *testing.T) {
	const toolName = "lookup"
	const toolResult = "검색 결과"
	const finalText = "최종 답"

	ft := &fakeTool{name: toolName, result: toolResult}
	reg := tool.NewRegistry()
	if err := reg.Register(ft); err != nil {
		t.Fatalf("tool 등록 실패: %v", err)
	}

	stub := &seqStub{
		responses: []llm.ChatResponse{
			toolCallResponse(),      // 1회전: tool_call 응답
			textResponse(finalText), // 2회전: text 응답 → StatusFinal
		},
	}
	a := agent.NewAgent(stub, "stub-model", 5, nil, reg, 0)
	state := a.Run(context.Background(), "tool 실행 테스트")

	// 종료 상태 검증
	if state.Status != agent.StatusFinal {
		t.Errorf("종료 상태 기대값 StatusFinal, 실제값 %q", state.Status)
	}

	// tool.Execute가 실제 호출됐는지 검증
	if ft.calls != 1 {
		t.Errorf("fakeTool.Execute 호출 횟수 기대값 1, 실제값 %d", ft.calls)
	}

	// LLM 호출 횟수 검증(tool_call 1회 + text 1회 = 2회)
	if stub.calls != 2 {
		t.Errorf("LLM 호출 횟수 기대값 2, 실제값 %d", stub.calls)
	}

	// 누적 메시지 구조 검증: user + assistant(tool_call) + tool(tool_result) + assistant(text)
	// 총 4개 메시지
	if len(state.Messages) != 4 {
		t.Fatalf("누적 메시지 수 기대값 4, 실제값 %d", len(state.Messages))
	}
	if state.Messages[0].Role != message.RoleUser {
		t.Errorf("Messages[0].Role 기대값 user, 실제값 %q", state.Messages[0].Role)
	}
	if state.Messages[1].Role != message.RoleAssistant {
		t.Errorf("Messages[1].Role 기대값 assistant, 실제값 %q", state.Messages[1].Role)
	}
	// tool_result 메시지 검증
	toolMsg := state.Messages[2]
	if toolMsg.Role != message.RoleTool {
		t.Errorf("Messages[2].Role 기대값 tool, 실제값 %q", toolMsg.Role)
	}
	if len(toolMsg.Content) == 0 {
		t.Fatal("RoleTool 메시지의 Content가 비어 있다")
	}
	if toolMsg.Content[0].Type != message.BlockTypeToolResult {
		t.Errorf("RoleTool 메시지 블록 Type 기대값 tool_result, 실제값 %q", toolMsg.Content[0].Type)
	}
	if toolMsg.Content[0].ToolResult == nil {
		t.Fatal("RoleTool 메시지 블록의 ToolResult가 nil이다")
	}
	if toolMsg.Content[0].ToolResult.Content != toolResult {
		t.Errorf("ToolResult.Content 기대값 %q, 실제값 %q", toolResult, toolMsg.Content[0].ToolResult.Content)
	}
	if toolMsg.Content[0].ToolResult.ToolCallID != "call_001" {
		t.Errorf("ToolResult.ToolCallID 기대값 %q, 실제값 %q", "call_001", toolMsg.Content[0].ToolResult.ToolCallID)
	}
	if state.Messages[3].Role != message.RoleAssistant {
		t.Errorf("Messages[3].Role 기대값 assistant, 실제값 %q", state.Messages[3].Role)
	}
}

// TestRun_ToolCall_ChatRequestContainsSpecs 는 ChatRequest.Tools가 등록 schema로 채워져
// LLM에 전달되는지 검증한다.
func TestRun_ToolCall_ChatRequestContainsSpecs(t *testing.T) {
	const toolName = "lookup"

	ft := &fakeTool{name: toolName, result: "ok"}
	reg := tool.NewRegistry()
	if err := reg.Register(ft); err != nil {
		t.Fatalf("tool 등록 실패: %v", err)
	}

	capturer := &capturingStub{
		seqStub: seqStub{
			responses: []llm.ChatResponse{
				toolCallResponse(),
				textResponse("완료"),
			},
		},
	}
	a := agent.NewAgent(capturer, "stub-model", 5, nil, reg, 0)
	a.Run(context.Background(), "spec 전달 테스트")

	// 첫 번째 ChatRequest에 Tools가 포함돼야 한다
	if len(capturer.capturedRequests) == 0 {
		t.Fatal("ChatRequest가 캡처되지 않았다")
	}
	firstReq := capturer.capturedRequests[0]
	if len(firstReq.Tools) == 0 {
		t.Fatal("ChatRequest.Tools가 비어 있다 — registry Specs가 전달되지 않았다")
	}
	if firstReq.Tools[0].Name != toolName {
		t.Errorf("ChatRequest.Tools[0].Name 기대값 %q, 실제값 %q", toolName, firstReq.Tools[0].Name)
	}
}

func TestRun_Phase51ToolBundle_SpecsResultsAndFinal(t *testing.T) {
	workDir := t.TempDir()
	reg := newPhase51TestRegistry(t, workDir)
	capturer := &capturingStub{
		seqStub: seqStub{
			responses: []llm.ChatResponse{
				phase51ToolBundleResponse(),
				textResponse("Phase 5.1 tool 묶음 완료"),
			},
		},
	}
	a := agent.NewAgent(capturer, "stub-model", 5, nil, reg, time.Second)
	state := a.Run(context.Background(), "Phase 5.1 tool 묶음 테스트")

	if state.Status != agent.StatusFinal {
		t.Fatalf("종료 상태 기대값 StatusFinal, 실제값 %q", state.Status)
	}
	if state.Err != nil {
		t.Fatalf("tool 실패는 Agent error가 아니어야 하나 Err=%v", state.Err)
	}
	if len(capturer.capturedRequests) == 0 {
		t.Fatal("ChatRequest가 캡처되지 않았다")
	}
	assertToolSpecs(t, capturer.capturedRequests[0].Tools, "web_search", "file_save", "code_execute")

	if len(state.Messages) != 4 {
		t.Fatalf("누적 메시지 수 기대값 4, 실제값 %d", len(state.Messages))
	}
	toolMsg := state.Messages[2]
	if toolMsg.Role != message.RoleTool {
		t.Fatalf("Messages[2].Role 기대값 tool, 실제값 %q", toolMsg.Role)
	}
	results := make(map[string]message.ToolResult)
	for _, block := range toolMsg.Content {
		if block.Type != message.BlockTypeToolResult || block.ToolResult == nil {
			t.Fatalf("RoleTool 메시지에 tool_result가 아닌 블록이 있다: %#v", block)
		}
		results[block.ToolResult.ToolCallID] = *block.ToolResult
	}
	if len(results) != 3 {
		t.Fatalf("tool result 수 기대값 3, 실제값 %d", len(results))
	}

	webResult := results["call_web"]
	if !webResult.IsError || !strings.Contains(webResult.Content, "TAVILY_API_KEY") {
		t.Errorf("web_search 설정 누락은 IsError result로 남아야 한다: %#v", webResult)
	}
	saveResult := results["call_save"]
	if saveResult.IsError || !strings.Contains(saveResult.Content, "saved path=") {
		t.Errorf("file_save 성공 result가 기대와 다르다: %#v", saveResult)
	}
	codeResult := results["call_code"]
	if codeResult.IsError || !strings.Contains(codeResult.Content, "agent code ok") {
		t.Errorf("code_execute 성공 result가 기대와 다르다: %#v", codeResult)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "agent-output.txt"))
	if err != nil {
		t.Fatalf("file_save 결과 파일 읽기 실패: %v", err)
	}
	if string(data) != "saved by agent regression" {
		t.Errorf("file_save 내용 기대값과 다름: %q", string(data))
	}
}

func assertToolSpecs(t *testing.T, specs []message.ToolSpec, expected ...string) {
	t.Helper()
	names := make(map[string]bool)
	for _, spec := range specs {
		names[spec.Name] = true
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("ChatRequest.Tools에 %q schema가 없다", name)
		}
	}
}

// TestRun_ToolCall_NilRegistry 는 registry가 nil이면 tool_call 응답을 신호로만 취급하고
// tool 실행 없이 running 유지→max steps로 안전 종료되는지 검증한다(기존 동작 보존).
func TestRun_ToolCall_NilRegistry(t *testing.T) {
	const maxSteps = 2

	stub := &seqStub{
		responses: []llm.ChatResponse{toolCallResponse()},
	}
	// registry를 명시적으로 nil로 전달
	a := agent.NewAgent(stub, "stub-model", maxSteps, nil, nil, 0)
	state := a.Run(context.Background(), "nil registry 테스트")

	// tool 실행 없이 max steps로 종료돼야 한다
	if state.Status != agent.StatusMaxSteps {
		t.Errorf("종료 상태 기대값 StatusMaxSteps, 실제값 %q", state.Status)
	}
	if stub.calls != maxSteps {
		t.Errorf("LLM 호출 횟수 기대값 %d, 실제값 %d", maxSteps, stub.calls)
	}
	// RoleTool 메시지가 없어야 한다
	for _, msg := range state.Messages {
		if msg.Role == message.RoleTool {
			t.Error("nil registry에서 RoleTool 메시지가 누적됐다 — tool이 실행되서는 안 된다")
		}
	}
}

// TestRun_ToolCall_IsErrorResultAccumulated 는 tool 실행 중 IsError=true 결과가
// tool_result로 정상 누적되고 loop가 이어지며 error 상태로 가지 않음을 검증한다.
func TestRun_ToolCall_IsErrorResultAccumulated(t *testing.T) {
	// "unknown_tool"은 registry에 없으므로 dispatcher가 IsError=true 결과를 반환한다
	unknownToolResp := llm.ChatResponse{
		Message: message.Message{
			Role: message.RoleAssistant,
			Content: []message.ContentBlock{
				message.NewToolCallBlock(message.ToolCall{
					ID:    "call_err",
					Name:  "unknown_tool",
					Input: json.RawMessage(`{}`),
				}),
			},
		},
	}

	stub := &seqStub{
		responses: []llm.ChatResponse{
			unknownToolResp,
			textResponse("에러 흡수 후 최종 답"),
		},
	}
	reg := tool.NewRegistry() // 빈 registry — unknown_tool이 없으므로 IsError 결과
	a := agent.NewAgent(stub, "stub-model", 5, nil, reg, 0)
	state := a.Run(context.Background(), "IsError 흡수 테스트")

	// tool 실패는 StatusError가 아닌 StatusFinal로 종료돼야 한다
	if state.Status != agent.StatusFinal {
		t.Errorf("종료 상태 기대값 StatusFinal, 실제값 %q", state.Status)
	}
	if state.Err != nil {
		t.Errorf("tool 실패는 error를 상위로 throw하지 않아야 하나 Err=%v", state.Err)
	}

	// RoleTool 메시지가 누적돼야 한다
	foundToolMsg := false
	for _, msg := range state.Messages {
		if msg.Role == message.RoleTool {
			foundToolMsg = true
			if len(msg.Content) > 0 && msg.Content[0].ToolResult != nil {
				if !msg.Content[0].ToolResult.IsError {
					t.Error("unknown tool 결과에서 IsError=true를 기대했으나 false다")
				}
			}
		}
	}
	if !foundToolMsg {
		t.Error("IsError 결과가 담긴 RoleTool 메시지가 누적되지 않았다")
	}
}

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
