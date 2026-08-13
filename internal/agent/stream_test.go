package agent

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	runtimetool "github.com/zipkero/agent-runtime/internal/tool"
)

// stubStreamStep 구조체는 stubStreamingClient의 StreamChat 호출 한 번이 내보내는 event 순서를 나타낸다.
// err가 설정되면 deltas를 모두 내보낸 뒤 오류 하나로 끝내고 response는 내보내지 않는다.
type stubStreamStep struct {
	deltas   []string
	response *llm.ChatResponse
	err      error
}

// stubStreamingClient는 llm.StreamingLLMClient를 구현해 여러 model 호출의 text delta와 완성 응답을 순서대로
// 재현하는 테스트 double이다. Chat은 streaming 경로 테스트에서 호출되지 않아야 하므로 호출되면 실패시킨다.
type stubStreamingClient struct {
	steps    []stubStreamStep
	calls    int
	requests []llm.ChatRequest
}

func (c *stubStreamingClient) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, errors.New("stubStreamingClient: Chat must not be called from a streaming path")
}

func (c *stubStreamingClient) StreamChat(_ context.Context, req llm.ChatRequest) iter.Seq2[llm.ChatStreamEvent, error] {
	return func(yield func(llm.ChatStreamEvent, error) bool) {
		idx := c.calls
		c.calls++
		c.requests = append(c.requests, llm.ChatRequest{
			Model:    req.Model,
			Messages: append([]message.Message(nil), req.Messages...),
			Tools:    append([]message.ToolSchema(nil), req.Tools...),
		})
		if idx >= len(c.steps) {
			return
		}
		step := c.steps[idx]
		for _, delta := range step.deltas {
			if !yield(llm.ChatStreamEvent{Kind: llm.ChatStreamEventTextDelta, TextDelta: delta}, nil) {
				return
			}
		}
		if step.err != nil {
			yield(llm.ChatStreamEvent{}, step.err)
			return
		}
		if step.response != nil {
			yield(llm.ChatStreamEvent{Kind: llm.ChatStreamEventResponse, Response: step.response}, nil)
		}
	}
}

// deadlineStubStreamingClient는 deadlineStubClient의 streaming 대응으로, 각 StreamChat 호출의 남은 deadline을
// 기록해 ModelTimeout이 streaming 경로에서도 호출마다 독립적으로 적용되는지 확인한다.
type deadlineStubStreamingClient struct {
	responses []llm.ChatResponse
	deadlines []time.Duration
}

func (c *deadlineStubStreamingClient) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, errors.New("deadlineStubStreamingClient: Chat must not be called from a streaming path")
}

func (c *deadlineStubStreamingClient) StreamChat(ctx context.Context, _ llm.ChatRequest) iter.Seq2[llm.ChatStreamEvent, error] {
	return func(yield func(llm.ChatStreamEvent, error) bool) {
		deadline, ok := ctx.Deadline()
		if !ok {
			yield(llm.ChatStreamEvent{}, errors.New("model context has no deadline"))
			return
		}
		c.deadlines = append(c.deadlines, time.Until(deadline))
		response := c.responses[0]
		c.responses = c.responses[1:]
		yield(llm.ChatStreamEvent{Kind: llm.ChatStreamEventResponse, Response: &response}, nil)
	}
}

// contextStubStreamingClient는 contextStubClient의 streaming 대응으로, 호출자 context가 끝날 때까지 응답을
// 만들지 않아 caller cancellation과 run deadline이 provider 오류로 잘못 분류되지 않는지 확인한다.
type contextStubStreamingClient struct {
	calls int
}

func (c *contextStubStreamingClient) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, errors.New("contextStubStreamingClient: Chat must not be called from a streaming path")
}

func (c *contextStubStreamingClient) StreamChat(ctx context.Context, _ llm.ChatRequest) iter.Seq2[llm.ChatStreamEvent, error] {
	return func(yield func(llm.ChatStreamEvent, error) bool) {
		c.calls++
		<-ctx.Done()
		yield(llm.ChatStreamEvent{}, ctx.Err())
	}
}

func collectTextDeltas(step int, textDelta string, deltas *[]RunnerStreamEvent) bool {
	*deltas = append(*deltas, RunnerStreamEvent{Kind: RunnerStreamEventTextDelta, Step: step, TextDelta: textDelta})
	return true
}

// TestAgentRunStreamDeliversDeltaOrderAcrossToolLoopSteps 는 streaming caller가 여러 step의 text delta를
// step과 생성 순서를 보존해 전달하고, 완성된 Tool call만 기존 Tool loop로 실행해 non-streaming Run과 같은
// 최종 상태에 도달하는지 확인한다.
func TestAgentRunStreamDeliversDeltaOrderAcrossToolLoopSteps(t *testing.T) {
	toolCall := message.ToolCall{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{"query":"runtime"}`)}
	client := &stubStreamingClient{steps: []stubStreamStep{
		{
			deltas:   []string{"check", "ing"},
			response: &llm.ChatResponse{Message: message.Assistant("checking", toolCall)},
		},
		{
			deltas:   []string{"final", " answer"},
			response: &llm.ChatResponse{Message: message.Assistant("final answer")},
		},
	}}
	lookupTool := &stubTool{name: "lookup", result: runtimetool.Result{Content: "tool output"}}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(lookupTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	agent, err := New(Options{Client: client, Model: "test-model", MaxSteps: 3, Tools: registry})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var deltas []RunnerStreamEvent
	onText := func(step int, textDelta string) bool {
		return collectTextDeltas(step, textDelta, &deltas)
	}

	state, aborted := agent.runStream(context.Background(), "use lookup", client, onText)

	if aborted {
		t.Fatal("aborted = true, want completed run")
	}
	if state.Status != StatusFinal || state.FinalAnswer != "final answer" {
		t.Fatalf("state = %+v, want final answer", state)
	}
	if client.calls != 2 || lookupTool.calls != 1 {
		t.Fatalf("client/tool calls = %d/%d, want 2/1", client.calls, lookupTool.calls)
	}
	wantDeltas := []RunnerStreamEvent{
		{Kind: RunnerStreamEventTextDelta, Step: 1, TextDelta: "check"},
		{Kind: RunnerStreamEventTextDelta, Step: 1, TextDelta: "ing"},
		{Kind: RunnerStreamEventTextDelta, Step: 2, TextDelta: "final"},
		{Kind: RunnerStreamEventTextDelta, Step: 2, TextDelta: " answer"},
	}
	if len(deltas) != len(wantDeltas) {
		t.Fatalf("len(deltas) = %d, want %d: %+v", len(deltas), len(wantDeltas), deltas)
	}
	for i, want := range wantDeltas {
		if deltas[i] != want {
			t.Fatalf("deltas[%d] = %+v, want %+v", i, deltas[i], want)
		}
	}
}

// TestAgentRunStreamAppliesMiddlewareInOrderAndUsesFinalResponse 는 streaming 경로에서도 pre/post-model
// middleware가 등록 순서대로 실행되고, post-model이 바꾼 완성 응답만 Agent 상태에 반영되는지 확인한다.
func TestAgentRunStreamAppliesMiddlewareInOrderAndUsesFinalResponse(t *testing.T) {
	client := &stubStreamingClient{steps: []stubStreamStep{
		{deltas: []string{"draft"}, response: &llm.ChatResponse{Message: message.Assistant("draft")}},
	}}
	var order []string
	middleware := []ModelMiddleware{
		{
			Name: "first",
			PreModel: func(_ context.Context, req llm.ChatRequest) (llm.ChatRequest, error) {
				order = append(order, "pre:first")
				return req, nil
			},
			PostModel: func(_ context.Context, _ llm.ChatRequest, resp llm.ChatResponse) (llm.ChatResponse, error) {
				order = append(order, "post:first")
				resp.Message.Text += " from first"
				return resp, nil
			},
		},
		{
			Name: "second",
			PreModel: func(_ context.Context, req llm.ChatRequest) (llm.ChatRequest, error) {
				order = append(order, "pre:second")
				return req, nil
			},
			PostModel: func(_ context.Context, _ llm.ChatRequest, resp llm.ChatResponse) (llm.ChatResponse, error) {
				order = append(order, "post:second")
				resp.Message.Text += " and second"
				return resp, nil
			},
		},
	}
	agent, err := newAgent(Options{Client: client, MaxSteps: 1}, modelCallOptions{middleware: middleware})
	if err != nil {
		t.Fatalf("newAgent() error = %v", err)
	}

	state, aborted := agent.runStream(context.Background(), "hello", client, nil)

	if aborted {
		t.Fatal("aborted = true, want completed run")
	}
	wantOrder := []string{"pre:first", "pre:second", "post:first", "post:second"}
	if len(order) != len(wantOrder) {
		t.Fatalf("order = %v, want %v", order, wantOrder)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("order = %v, want %v", order, wantOrder)
		}
	}
	if state.Status != StatusFinal || state.FinalAnswer != "draft from first and second" {
		t.Fatalf("state = %+v, want middleware-modified final answer", state)
	}
}

// TestAgentRunStreamStopsAtMiddlewareFailureWithoutFurtherModelCalls 는 middleware 실패 이후 추가 model 호출이
// 없고 오류가 상태에 보존되는지 확인한다.
func TestAgentRunStreamStopsAtMiddlewareFailureWithoutFurtherModelCalls(t *testing.T) {
	sentinel := errors.New("middleware failed")
	client := &stubStreamingClient{steps: []stubStreamStep{
		{response: &llm.ChatResponse{Message: message.Assistant("should not be used")}},
	}}
	middleware := ModelMiddleware{
		Name: "pre-failure",
		PreModel: func(context.Context, llm.ChatRequest) (llm.ChatRequest, error) {
			return llm.ChatRequest{}, sentinel
		},
	}
	agent, err := newAgent(Options{Client: client, MaxSteps: 3}, modelCallOptions{middleware: []ModelMiddleware{middleware}})
	if err != nil {
		t.Fatalf("newAgent() error = %v", err)
	}

	state, aborted := agent.runStream(context.Background(), "hello", client, nil)

	if aborted {
		t.Fatal("aborted = true, want middleware error instead")
	}
	if state.Status != StatusError || !errors.Is(state.LastError, sentinel) {
		t.Fatalf("state = %+v, want middleware error", state)
	}
	if client.calls != 0 {
		t.Fatalf("client calls = %d, want 0", client.calls)
	}
}

// TestAgentRunStreamRejectsIncompleteFinishReasons 는 길이 제한, 차단, 알 수 없는 완료 사유가 이미 전달된 delta와
// 무관하게 정상 final로 승격되지 않는지 확인한다.
func TestAgentRunStreamRejectsIncompleteFinishReasons(t *testing.T) {
	tests := []struct {
		name         string
		finishReason llm.FinishReason
		stopReason   string
	}{
		{name: "length limit", finishReason: llm.FinishReasonLengthLimit, stopReason: "max_tokens"},
		{name: "blocked", finishReason: llm.FinishReasonBlocked, stopReason: "refusal"},
		{name: "unknown", finishReason: llm.FinishReasonUnknown, stopReason: "future_reason"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &stubStreamingClient{steps: []stubStreamStep{
				{
					deltas: []string{"partial"},
					response: &llm.ChatResponse{
						Message:      message.Assistant("partial response"),
						FinishReason: tt.finishReason,
						StopReason:   tt.stopReason,
					},
				},
			}}
			agent, err := New(Options{Client: client, MaxSteps: 2})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			var deltas []string
			state, aborted := agent.runStream(context.Background(), "hello", client, func(_ int, textDelta string) bool {
				deltas = append(deltas, textDelta)
				return true
			})

			if aborted {
				t.Fatal("aborted = true, want incomplete response error")
			}
			var incompleteErr *RunnerError
			if state.Status != StatusError || state.FinalAnswer != "" ||
				!errors.As(state.LastError, &incompleteErr) || incompleteErr.Kind != RunnerErrorKindIncompleteResponse {
				t.Fatalf("state = %+v, want incomplete response error", state)
			}
			if len(deltas) != 1 || deltas[0] != "partial" {
				t.Fatalf("deltas = %v, want provisional delta preserved", deltas)
			}
			if client.calls != 1 {
				t.Fatalf("client calls = %d, want 1", client.calls)
			}
		})
	}
}

// TestAgentRunStreamReturnsProviderErrorFromStream 는 stream 도중 발생한 공급자 오류가 이미 전달된 delta와
// 별개로 LLM 오류로 분류되고, 이후 model 호출이 없는지 확인한다.
func TestAgentRunStreamReturnsProviderErrorFromStream(t *testing.T) {
	streamErr := errors.New("stream disconnected")
	client := &stubStreamingClient{steps: []stubStreamStep{
		{deltas: []string{"partial"}, err: streamErr},
	}}
	agent, err := New(Options{Client: client, MaxSteps: 2})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state, aborted := agent.runStream(context.Background(), "hello", client, nil)

	if aborted {
		t.Fatal("aborted = true, want provider error instead")
	}
	if state.Status != StatusError || !errors.Is(state.LastError, streamErr) {
		t.Fatalf("state = %+v, want provider stream error", state)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
}

// TestAgentRunStreamAppliesExecutionLimitsAcrossSteps 는 streaming 경로에서도 max Tool 호출 상한이 non-streaming과
// 같은 방식으로 실행을 끊는지 확인한다.
func TestAgentRunStreamAppliesExecutionLimitsAcrossSteps(t *testing.T) {
	calls := make([]message.ToolCall, defaultMaxToolCalls+1)
	for i := range calls {
		calls[i] = message.ToolCall{ID: "call", Name: "lookup", Arguments: json.RawMessage(`{}`)}
	}
	client := &stubStreamingClient{steps: []stubStreamStep{
		{response: &llm.ChatResponse{Message: message.Assistant("many calls", calls...)}},
	}}
	lookupTool := &stubTool{name: "lookup", result: runtimetool.Result{Content: "ok"}}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(lookupTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	agent, err := New(Options{Client: client, MaxSteps: 2, Tools: registry})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state, aborted := agent.runStream(context.Background(), "use many tools", client, nil)

	if aborted {
		t.Fatal("aborted = true, want execution limit error")
	}
	var limitErr *RunnerError
	if state.Status != StatusError || client.calls != 1 || lookupTool.calls != defaultMaxToolCalls ||
		!errors.As(state.LastError, &limitErr) || limitErr.Kind != RunnerErrorKindExecutionLimit || limitErr.Limit != limitMaxToolCalls {
		t.Fatalf("state/client/tool calls = %+v/%d/%d, want max tool calls execution limit", state, client.calls, lookupTool.calls)
	}
}

// TestAgentRunStreamAppliesIndependentModelTimeoutPerCall 는 ModelTimeout이 streaming 호출마다 새로 주어지는지
// non-streaming Runner 테스트와 같은 방식으로 확인한다.
func TestAgentRunStreamAppliesIndependentModelTimeoutPerCall(t *testing.T) {
	client := &deadlineStubStreamingClient{responses: []llm.ChatResponse{{Message: message.Assistant("answer")}}}
	agent, err := newAgent(Options{Client: client, MaxSteps: 1}, modelCallOptions{timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("newAgent() error = %v", err)
	}

	state, aborted := agent.runStream(context.Background(), "hello", client, nil)

	if aborted {
		t.Fatal("aborted = true, want completed run")
	}
	if state.Status != StatusFinal || state.FinalAnswer != "answer" {
		t.Fatalf("state = %+v, want final answer", state)
	}
	if len(client.deadlines) != 1 {
		t.Fatalf("len(deadlines) = %d, want provider deadline", len(client.deadlines))
	}
	if client.deadlines[0] < 80*time.Millisecond || client.deadlines[0] > 100*time.Millisecond {
		t.Fatalf("deadline remaining = %v, want near 100ms", client.deadlines[0])
	}
}

// TestAgentRunStreamClassifiesCallerDeadlineAsExecutionLimit 는 호출자 deadline 초과가 provider 오류가 아니라
// 실행 제한으로 분류되는지 확인한다.
func TestAgentRunStreamClassifiesCallerDeadlineAsExecutionLimit(t *testing.T) {
	client := &contextStubStreamingClient{}
	agent, err := New(Options{Client: client, MaxSteps: 1})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	state, aborted := agent.runStream(ctx, "wait", client, nil)

	var limitErr *RunnerError
	if aborted || state.Status != StatusError || !errors.As(state.LastError, &limitErr) ||
		limitErr.Limit != limitRunDeadline || !errors.Is(state.LastError, context.DeadlineExceeded) {
		t.Fatalf("state/aborted = %+v/%v, want run deadline execution limit", state, aborted)
	}
}

// TestAgentRunStreamStopsWithoutFurtherCallsWhenSinkAborts 는 소비자가 text delta 소비를 중단하면 이후 model
// 호출이나 Tool 실행 없이 loop가 즉시 멈추는지 확인한다.
func TestAgentRunStreamStopsWithoutFurtherCallsWhenSinkAborts(t *testing.T) {
	toolCall := message.ToolCall{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{}`)}
	client := &stubStreamingClient{steps: []stubStreamStep{
		{deltas: []string{"first", "second"}, response: &llm.ChatResponse{Message: message.Assistant("checking", toolCall)}},
		{deltas: []string{"unused"}, response: &llm.ChatResponse{Message: message.Assistant("final")}},
	}}
	lookupTool := &stubTool{name: "lookup", result: runtimetool.Result{Content: "ok"}}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(lookupTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	agent, err := New(Options{Client: client, MaxSteps: 3, Tools: registry})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	seen := 0
	state, aborted := agent.runStream(context.Background(), "use lookup", client, func(int, string) bool {
		seen++
		return seen < 1
	})

	if !aborted {
		t.Fatalf("aborted = false, want abort after first delta")
	}
	if seen != 1 {
		t.Fatalf("seen deltas = %d, want 1", seen)
	}
	if client.calls != 1 || lookupTool.calls != 0 {
		t.Fatalf("client/tool calls = %d/%d, want 1/0", client.calls, lookupTool.calls)
	}
	_ = state
}

// collectRunnerStreamEvents 함수는 breakAfter 이벤트를 소비한 뒤 순회를 중단하고, breakAfter가 0 이하이면 끝까지
// 소비한다.
func collectRunnerStreamEvents(seq iter.Seq[RunnerStreamEvent], breakAfter int) []RunnerStreamEvent {
	var events []RunnerStreamEvent
	for event := range seq {
		events = append(events, event)
		if breakAfter > 0 && len(events) >= breakAfter {
			break
		}
	}
	return events
}

// TestRunnerRunStreamDeliversDeltasThenFinal 는 완전 소비 시 text delta 뒤 정확히 한 번의 final event만 나오는지
// 확인한다.
func TestRunnerRunStreamDeliversDeltasThenFinal(t *testing.T) {
	client := &stubStreamingClient{steps: []stubStreamStep{
		{deltas: []string{"hel", "lo"}, response: &llm.ChatResponse{Message: message.Assistant("hello")}},
	}}
	runner, err := NewRunner(RunnerOptions{Client: client, MaxSteps: 1})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	events := collectRunnerStreamEvents(runner.RunStream(context.Background(), "hi"), 0)

	if len(events) != 3 {
		t.Fatalf("len(events) = %d, want 2 delta + 1 final: %+v", len(events), events)
	}
	for i, want := range []string{"hel", "lo"} {
		if events[i].Kind != RunnerStreamEventTextDelta || events[i].Step != 1 || events[i].TextDelta != want {
			t.Fatalf("events[%d] = %+v, want text_delta step=1 %q", i, events[i], want)
		}
	}
	final := events[2]
	if final.Kind != RunnerStreamEventFinal || final.Result == nil ||
		final.Result.State.Status != StatusFinal || final.Result.State.FinalAnswer != "hello" {
		t.Fatalf("final event = %+v, want final result", final)
	}
}

// TestRunnerRunStreamReturnsErrorEventForProviderFailure 는 provider stream 오류가 error event 하나로 끝나고
// final event를 만들지 않는지 확인한다.
func TestRunnerRunStreamReturnsErrorEventForProviderFailure(t *testing.T) {
	sentinel := errors.New("provider unavailable")
	client := &stubStreamingClient{steps: []stubStreamStep{{err: sentinel}}}
	runner, err := NewRunner(RunnerOptions{Client: client, MaxSteps: 1})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	events := collectRunnerStreamEvents(runner.RunStream(context.Background(), "hi"), 0)

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want single error event: %+v", len(events), events)
	}
	got := events[0]
	if got.Kind != RunnerStreamEventError || got.Result == nil ||
		got.Result.State.Status != StatusError || !errors.Is(got.Result.State.LastError, sentinel) {
		t.Fatalf("event = %+v, want provider error result", got)
	}
}

// TestRunnerRunStreamReturnsErrorEventForUnsupportedStreamingClient 는 streaming을 지원하지 않는 client로
// RunStream을 호출하면 provider 호출 없이 error event 한 번으로 끝나고 기존 Run은 계속 동작하는지 확인한다.
func TestRunnerRunStreamReturnsErrorEventForUnsupportedStreamingClient(t *testing.T) {
	client := &stubClient{response: llm.ChatResponse{Message: message.Assistant("answer")}}
	runner, err := NewRunner(RunnerOptions{Client: client, MaxSteps: 1})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	events := collectRunnerStreamEvents(runner.RunStream(context.Background(), "hi"), 0)

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want single error event: %+v", len(events), events)
	}
	got := events[0]
	if got.Kind != RunnerStreamEventError || got.Result == nil || got.Result.State.Status != StatusError ||
		!IsRunnerErrorKind(got.Result.State.LastError, RunnerErrorKindUnsupportedStream) {
		t.Fatalf("event = %+v, want unsupported stream error", got)
	}
	if client.calls != 0 {
		t.Fatalf("client calls = %d, want 0", client.calls)
	}

	// 기존 non-streaming Run 호출은 같은 client에서 변경 없이 계속 동작해야 한다.
	result := runner.Run(context.Background(), "hi")
	if result.State.Status != StatusFinal || result.State.FinalAnswer != "answer" {
		t.Fatalf("Run() result = %+v, want unaffected non-streaming behavior", result)
	}
}

// TestRunnerRunStreamStopsWithoutTerminalEventWhenConsumerBreaks 는 소비자가 순회를 중단하면 그 시점 이후
// provider 요청이나 terminal event가 발생하지 않는지 확인한다.
func TestRunnerRunStreamStopsWithoutTerminalEventWhenConsumerBreaks(t *testing.T) {
	toolCall := message.ToolCall{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{}`)}
	client := &stubStreamingClient{steps: []stubStreamStep{
		{deltas: []string{"a", "b"}, response: &llm.ChatResponse{Message: message.Assistant("checking", toolCall)}},
		{deltas: []string{"c"}, response: &llm.ChatResponse{Message: message.Assistant("final")}},
	}}
	lookupTool := &stubTool{name: "lookup", result: runtimetool.Result{Content: "ok"}}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(lookupTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	runner, err := NewRunner(RunnerOptions{Client: client, MaxSteps: 3, Tools: registry})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	events := collectRunnerStreamEvents(runner.RunStream(context.Background(), "use lookup"), 1)

	if len(events) != 1 || events[0].Kind != RunnerStreamEventTextDelta {
		t.Fatalf("events = %+v, want single text delta before consumer stopped", events)
	}
	if client.calls != 1 || lookupTool.calls != 0 {
		t.Fatalf("client/tool calls = %d/%d, want 1/0 after early consumer stop", client.calls, lookupTool.calls)
	}
}

// TestRunnerRunStreamClassifiesCallerCancellationAsError 는 호출자가 context를 취소하면 provider 요청이 정리되고
// 취소가 최종 오류로 확인되는지 확인한다.
func TestRunnerRunStreamClassifiesCallerCancellationAsError(t *testing.T) {
	client := &contextStubStreamingClient{}
	runner, err := NewRunner(RunnerOptions{Client: client, MaxSteps: 1})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	events := collectRunnerStreamEvents(runner.RunStream(ctx, "hi"), 0)

	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want single error event: %+v", len(events), events)
	}
	got := events[0]
	if got.Kind != RunnerStreamEventError || got.Result == nil ||
		got.Result.State.Status != StatusError || !errors.Is(got.Result.State.LastError, context.Canceled) {
		t.Fatalf("event = %+v, want caller cancellation error", got)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1 canceled call", client.calls)
	}
}
