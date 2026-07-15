package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	runtimetool "github.com/zipkero/agent-runtime/internal/tool"
)

type stubClient struct {
	response  llm.ChatResponse
	responses []llm.ChatResponse
	err       error
	request   llm.ChatRequest
	requests  []llm.ChatRequest
	calls     int
	callStart chan int
}

func (c *stubClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls++
	if c.callStart != nil {
		c.callStart <- c.calls
	}
	c.request = llm.ChatRequest{
		Model:    req.Model,
		Messages: append([]message.Message(nil), req.Messages...),
		Tools:    append([]message.ToolSchema(nil), req.Tools...),
	}
	c.requests = append(c.requests, c.request)
	if c.err != nil {
		return llm.ChatResponse{}, c.err
	}
	if len(c.responses) > 0 {
		resp := c.responses[0]
		c.responses = c.responses[1:]
		return resp, nil
	}
	return c.response, nil
}

type stubTool struct {
	name          string
	description   string
	schema        message.ToolSchema
	result        runtimetool.Result
	validateErr   error
	executeErr    error
	waitForCtx    bool
	args          json.RawMessage
	validateCalls int
	calls         int
}

type controlledTool struct {
	result         runtimetool.Result
	executeErr     error
	waitForContext bool
	started        chan struct{}
	contextDone    chan struct{}
	release        chan struct{}
	returned       chan struct{}
}

func newControlledTool(result runtimetool.Result, executeErr error, waitForContext bool) *controlledTool {
	return &controlledTool{
		result:         result,
		executeErr:     executeErr,
		waitForContext: waitForContext,
		started:        make(chan struct{}),
		contextDone:    make(chan struct{}),
		release:        make(chan struct{}),
		returned:       make(chan struct{}),
	}
}

func (*controlledTool) Name() string {
	return "controlled"
}

func (*controlledTool) Description() string {
	return "Control Tool execution timing in tests."
}

func (*controlledTool) Schema() message.ToolSchema {
	return message.ToolSchema{Name: "controlled", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (*controlledTool) Validate(json.RawMessage) error {
	return nil
}

func (t *controlledTool) Execute(ctx context.Context, _ json.RawMessage) (runtimetool.Result, error) {
	close(t.started)
	if t.waitForContext {
		<-ctx.Done()
		close(t.contextDone)
	}
	<-t.release
	defer close(t.returned)

	if err := ctx.Err(); err != nil {
		return runtimetool.Result{}, err
	}
	if t.executeErr != nil {
		return runtimetool.Result{}, t.executeErr
	}
	return t.result, nil
}

func (t *stubTool) Name() string {
	return t.name
}

func (t *stubTool) Description() string {
	return t.description
}

func (t *stubTool) Schema() message.ToolSchema {
	return t.schema
}

func (t *stubTool) Validate(json.RawMessage) error {
	t.validateCalls++
	return t.validateErr
}

func (t *stubTool) Execute(ctx context.Context, args json.RawMessage) (runtimetool.Result, error) {
	t.calls++
	t.args = append(json.RawMessage(nil), args...)
	if t.waitForCtx {
		<-ctx.Done()
		return runtimetool.Result{}, ctx.Err()
	}
	if t.executeErr != nil {
		return runtimetool.Result{}, t.executeErr
	}
	return t.result, nil
}

type expectedTrace struct {
	step        int
	action      TraceAction
	status      Status
	toolCallID  string
	toolName    string
	isError     bool
	wantErr     error
	wantErrText string
}

func assertTrace(t *testing.T, got []TraceEvent, want []expectedTrace) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("len(Trace) = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Step != want[i].step || got[i].Action != want[i].action || got[i].Status != want[i].status {
			t.Fatalf("Trace[%d] = %+v, want step=%d action=%q status=%q", i, got[i], want[i].step, want[i].action, want[i].status)
		}
		if got[i].ToolCallID != want[i].toolCallID || got[i].ToolName != want[i].toolName || got[i].IsError != want[i].isError {
			t.Fatalf("Trace[%d] tool fields = %+v, want id=%q name=%q isError=%v", i, got[i], want[i].toolCallID, want[i].toolName, want[i].isError)
		}
		if want[i].wantErrText != "" {
			if got[i].Error == nil || got[i].Error.Error() != want[i].wantErrText {
				t.Fatalf("Trace[%d].Error = %v, want text %q", i, got[i].Error, want[i].wantErrText)
			}
			continue
		}
		if !errors.Is(got[i].Error, want[i].wantErr) {
			t.Fatalf("Trace[%d].Error = %v, want %v", i, got[i].Error, want[i].wantErr)
		}
	}
}

// TestRunStoresUserMessageAndFinalAssistantResponse 는 Task 001의 final 정상 경로 contract를 고정한다.
func TestRunStoresUserMessageAndFinalAssistantResponse(t *testing.T) {
	client := &stubClient{
		response: llm.ChatResponse{
			Message: message.Assistant("answer text"),
		},
	}
	agent, err := New(Options{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 3,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "hello runtime")

	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
	if client.request.Model != "test-model" {
		t.Fatalf("request Model = %q, want test-model", client.request.Model)
	}
	if len(client.request.Messages) != 1 {
		t.Fatalf("len(request Messages) = %d, want 1", len(client.request.Messages))
	}
	if client.request.Messages[0].Role != message.RoleUser || client.request.Messages[0].Text != "hello runtime" {
		t.Fatalf("request user message = %+v, want role=user text=hello runtime", client.request.Messages[0])
	}

	if state.Status != StatusFinal {
		t.Fatalf("state Status = %q, want %q", state.Status, StatusFinal)
	}
	if state.FinalAnswer != "answer text" {
		t.Fatalf("FinalAnswer = %q, want answer text", state.FinalAnswer)
	}
	if state.Step != 1 {
		t.Fatalf("Step = %d, want 1", state.Step)
	}
	if len(state.Messages) != 2 {
		t.Fatalf("len(state Messages) = %d, want 2", len(state.Messages))
	}
	if state.Messages[0].Role != message.RoleUser || state.Messages[0].Text != "hello runtime" {
		t.Fatalf("state user message = %+v, want role=user text=hello runtime", state.Messages[0])
	}
	if state.Messages[1].Role != message.RoleAssistant || state.Messages[1].Text != "answer text" {
		t.Fatalf("state assistant message = %+v, want role=assistant text=answer text", state.Messages[1])
	}
	assertTrace(t, state.Trace, []expectedTrace{
		{step: 0, action: TraceActionUserMessage, status: StatusRunning},
		{step: 1, action: TraceActionLLMRequest, status: StatusRunning},
		{step: 1, action: TraceActionLLMResponse, status: StatusRunning},
		{step: 1, action: TraceActionFinal, status: StatusFinal},
	})
}

// TestRunStopsWithNeedsActionWhenAssistantRequestsTool 는 tool 실행 없는 대기 상태 contract를 고정한다.
func TestRunStopsWithNeedsActionWhenAssistantRequestsTool(t *testing.T) {
	toolCall := message.ToolCall{
		ID:        "call-1",
		Name:      "read_file",
		Arguments: json.RawMessage(`{"path":"README.md"}`),
	}
	client := &stubClient{
		response: llm.ChatResponse{
			Message: message.Assistant("need tool", toolCall),
		},
	}
	agent, err := New(Options{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 3,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "read README")

	if state.Status != StatusNeedsAction {
		t.Fatalf("state Status = %q, want %q", state.Status, StatusNeedsAction)
	}
	if state.FinalAnswer != "" {
		t.Fatalf("FinalAnswer = %q, want empty", state.FinalAnswer)
	}
	if len(state.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(state.ToolCalls))
	}
	call := state.ToolCalls[0]
	if call.ID != "call-1" || call.Name != "read_file" || string(call.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("ToolCalls[0] = %+v, want call-1/read_file args", call)
	}
	if len(state.Messages) != 2 {
		t.Fatalf("len(state Messages) = %d, want 2", len(state.Messages))
	}
	assistant := state.Messages[1]
	if assistant.Role != message.RoleAssistant || assistant.Text != "need tool" {
		t.Fatalf("assistant message = %+v, want role=assistant text=need tool", assistant)
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("len(assistant ToolCalls) = %d, want 1", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].ID != call.ID ||
		assistant.ToolCalls[0].Name != call.Name ||
		string(assistant.ToolCalls[0].Arguments) != string(call.Arguments) {
		t.Fatalf("assistant ToolCalls = %+v, want same state call %+v", assistant.ToolCalls[0], call)
	}
	if assistant.ToolResult != nil {
		t.Fatalf("assistant ToolResult = %+v, want nil", assistant.ToolResult)
	}
	assertTrace(t, state.Trace, []expectedTrace{
		{step: 0, action: TraceActionUserMessage, status: StatusRunning},
		{step: 1, action: TraceActionLLMRequest, status: StatusRunning},
		{step: 1, action: TraceActionLLMResponse, status: StatusRunning},
		{step: 1, action: TraceActionNeedsAction, status: StatusNeedsAction},
	})
}

// TestRunExecutesRegisteredToolAndContinuesToFinal 는 registry가 있으면 tool result를 누적하고 다음 LLM 판단까지 이어가는지 확인한다.
func TestRunExecutesRegisteredToolAndContinuesToFinal(t *testing.T) {
	toolCall := message.ToolCall{
		ID:        "call-1",
		Name:      "lookup",
		Arguments: json.RawMessage(`{"query":"runtime"}`),
	}
	client := &stubClient{
		responses: []llm.ChatResponse{
			{Message: message.Assistant("checking", toolCall)},
			{Message: message.Assistant("final answer")},
		},
	}
	registry := runtimetool.NewRegistry()
	lookupTool := &stubTool{
		name:        "lookup",
		description: "Lookup runtime data",
		schema: message.ToolSchema{
			Name:        "lookup",
			Description: "Lookup runtime data",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
		},
		result: runtimetool.Result{Content: "tool output"},
	}
	if err := registry.Register(lookupTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	agent, err := New(Options{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 3,
		Tools:    registry,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "use lookup")

	if state.Status != StatusFinal {
		t.Fatalf("state Status = %q, want %q", state.Status, StatusFinal)
	}
	if state.FinalAnswer != "final answer" {
		t.Fatalf("FinalAnswer = %q, want final answer", state.FinalAnswer)
	}
	if len(state.ToolCalls) != 0 {
		t.Fatalf("len(ToolCalls) = %d, want none for final response", len(state.ToolCalls))
	}
	if state.Step != 2 {
		t.Fatalf("Step = %d, want 2", state.Step)
	}
	if client.calls != 2 {
		t.Fatalf("client calls = %d, want 2", client.calls)
	}
	if lookupTool.calls != 1 {
		t.Fatalf("tool calls = %d, want 1", lookupTool.calls)
	}
	if string(lookupTool.args) != `{"query":"runtime"}` {
		t.Fatalf("tool args = %s, want query args", lookupTool.args)
	}
	if len(client.requests) != 2 {
		t.Fatalf("len(client requests) = %d, want 2", len(client.requests))
	}
	for i, req := range client.requests {
		if len(req.Tools) != 1 {
			t.Fatalf("request[%d] len(Tools) = %d, want 1", i, len(req.Tools))
		}
		if req.Tools[0].Name != "lookup" || string(req.Tools[0].InputSchema) != string(lookupTool.schema.InputSchema) {
			t.Fatalf("request[%d] Tools[0] = %+v, want lookup schema", i, req.Tools[0])
		}
	}
	if len(client.requests[1].Messages) != 3 {
		t.Fatalf("second request len(Messages) = %d, want user, assistant, tool", len(client.requests[1].Messages))
	}
	toolMessage := client.requests[1].Messages[2]
	if toolMessage.Role != message.RoleTool || toolMessage.ToolResult == nil {
		t.Fatalf("second request third message = %+v, want tool result", toolMessage)
	}
	if toolMessage.ToolResult.ToolCallID != "call-1" ||
		toolMessage.ToolResult.Name != "lookup" ||
		toolMessage.ToolResult.Content != "tool output" ||
		toolMessage.ToolResult.IsError {
		t.Fatalf("ToolResult = %+v, want successful lookup result", toolMessage.ToolResult)
	}
	if len(state.Messages) != 4 {
		t.Fatalf("len(state Messages) = %d, want 4", len(state.Messages))
	}
	if state.Messages[2].Role != message.RoleTool || state.Messages[2].ToolResult == nil {
		t.Fatalf("state Messages[2] = %+v, want tool result", state.Messages[2])
	}
	if state.Messages[3].Role != message.RoleAssistant || state.Messages[3].Text != "final answer" {
		t.Fatalf("state final message = %+v, want assistant final answer", state.Messages[3])
	}
	assertTrace(t, state.Trace, []expectedTrace{
		{step: 0, action: TraceActionUserMessage, status: StatusRunning},
		{step: 1, action: TraceActionLLMRequest, status: StatusRunning},
		{step: 1, action: TraceActionLLMResponse, status: StatusRunning},
		{step: 1, action: TraceActionToolCall, status: StatusRunning, toolCallID: "call-1", toolName: "lookup"},
		{step: 1, action: TraceActionToolResult, status: StatusRunning, toolCallID: "call-1", toolName: "lookup"},
		{step: 2, action: TraceActionLLMRequest, status: StatusRunning},
		{step: 2, action: TraceActionLLMResponse, status: StatusRunning},
		{step: 2, action: TraceActionFinal, status: StatusFinal},
	})
}

// TestRunAppendsToolErrorResultAndContinues 는 Tool 실패가 Agent 오류가 아니라 오류 result로 다음 LLM 판단에 전달되는지 확인한다.
func TestRunAppendsToolErrorResultAndContinues(t *testing.T) {
	validationErr := errors.New("invalid arguments")
	executeErr := errors.New("tool failed")

	tests := []struct {
		name              string
		call              message.ToolCall
		tool              *stubTool
		wantContent       string
		wantToolCalls     int
		wantValidateCalls int
		wantTraceErr      error
		wantTraceErrText  string
	}{
		{
			name: "unknown tool",
			call: message.ToolCall{
				ID:        "call-unknown",
				Name:      "missing",
				Arguments: json.RawMessage(`{"query":"runtime"}`),
			},
			tool:             &stubTool{name: "other"},
			wantContent:      `tool "missing" is not registered`,
			wantTraceErrText: `tool "missing" is not registered`,
		},
		{
			name: "validation failure",
			call: message.ToolCall{
				ID:        "call-invalid",
				Name:      "lookup",
				Arguments: json.RawMessage(`{"query":123}`),
			},
			tool:              &stubTool{name: "lookup", validateErr: validationErr},
			wantContent:       validationErr.Error(),
			wantValidateCalls: 1,
			wantTraceErr:      validationErr,
		},
		{
			name: "execute error",
			call: message.ToolCall{
				ID:        "call-error",
				Name:      "lookup",
				Arguments: json.RawMessage(`{"query":"runtime"}`),
			},
			tool:              &stubTool{name: "lookup", executeErr: executeErr},
			wantContent:       executeErr.Error(),
			wantToolCalls:     1,
			wantValidateCalls: 1,
			wantTraceErr:      executeErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &stubClient{
				responses: []llm.ChatResponse{
					{Message: message.Assistant("checking", tt.call)},
					{Message: message.Assistant("final after error")},
				},
			}
			registry := runtimetool.NewRegistry()
			if err := registry.Register(tt.tool); err != nil {
				t.Fatalf("Register() error = %v", err)
			}
			agent, err := New(Options{
				Client:   client,
				Model:    "test-model",
				MaxSteps: 3,
				Tools:    registry,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			state := agent.Run(context.Background(), "use lookup")

			if state.Status != StatusFinal {
				t.Fatalf("state Status = %q, want %q", state.Status, StatusFinal)
			}
			if state.LastError != nil {
				t.Fatalf("LastError = %v, want nil", state.LastError)
			}
			if client.calls != 2 {
				t.Fatalf("client calls = %d, want 2", client.calls)
			}
			if tt.tool.calls != tt.wantToolCalls {
				t.Fatalf("tool calls = %d, want %d", tt.tool.calls, tt.wantToolCalls)
			}
			if tt.tool.validateCalls != tt.wantValidateCalls {
				t.Fatalf("validate calls = %d, want %d", tt.tool.validateCalls, tt.wantValidateCalls)
			}
			if len(client.requests[1].Messages) != 3 {
				t.Fatalf("second request len(Messages) = %d, want 3", len(client.requests[1].Messages))
			}
			toolMessage := client.requests[1].Messages[2]
			if toolMessage.Role != message.RoleTool || toolMessage.ToolResult == nil {
				t.Fatalf("second request third message = %+v, want tool result", toolMessage)
			}
			if toolMessage.ToolResult.ToolCallID != tt.call.ID ||
				toolMessage.ToolResult.Name != tt.call.Name ||
				toolMessage.ToolResult.Content != tt.wantContent ||
				!toolMessage.ToolResult.IsError {
				t.Fatalf("ToolResult = %+v, want error result for %s", toolMessage.ToolResult, tt.call.Name)
			}
			assertTrace(t, state.Trace, []expectedTrace{
				{step: 0, action: TraceActionUserMessage, status: StatusRunning},
				{step: 1, action: TraceActionLLMRequest, status: StatusRunning},
				{step: 1, action: TraceActionLLMResponse, status: StatusRunning},
				{step: 1, action: TraceActionToolCall, status: StatusRunning, toolCallID: tt.call.ID, toolName: tt.call.Name},
				{step: 1, action: TraceActionToolError, status: StatusRunning, toolCallID: tt.call.ID, toolName: tt.call.Name, isError: true, wantErr: tt.wantTraceErr, wantErrText: tt.wantTraceErrText},
				{step: 2, action: TraceActionLLMRequest, status: StatusRunning},
				{step: 2, action: TraceActionLLMResponse, status: StatusRunning},
				{step: 2, action: TraceActionFinal, status: StatusFinal},
			})
		})
	}
}

// TestRunAppendsToolTimeoutResultAndContinues 는 Tool timeout이 오류 result와 timeout trace로 보존되는지 확인한다.
func TestRunAppendsToolTimeoutResultAndContinues(t *testing.T) {
	toolCall := message.ToolCall{
		ID:        "call-timeout",
		Name:      "slow",
		Arguments: json.RawMessage(`{"query":"runtime"}`),
	}
	client := &stubClient{
		responses: []llm.ChatResponse{
			{Message: message.Assistant("checking", toolCall)},
			{Message: message.Assistant("final after timeout")},
		},
	}
	registry := runtimetool.NewRegistry()
	slowTool := &stubTool{name: "slow", waitForCtx: true}
	if err := registry.Register(slowTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	agent, err := New(Options{
		Client:      client,
		Model:       "test-model",
		MaxSteps:    3,
		Tools:       registry,
		ToolTimeout: 5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "use slow")

	if state.Status != StatusFinal {
		t.Fatalf("state Status = %q, want %q", state.Status, StatusFinal)
	}
	if client.calls != 2 {
		t.Fatalf("client calls = %d, want 2", client.calls)
	}
	if slowTool.calls != 1 {
		t.Fatalf("tool calls = %d, want 1", slowTool.calls)
	}
	toolMessage := client.requests[1].Messages[2]
	if toolMessage.Role != message.RoleTool || toolMessage.ToolResult == nil {
		t.Fatalf("second request third message = %+v, want tool result", toolMessage)
	}
	if toolMessage.ToolResult.ToolCallID != "call-timeout" ||
		toolMessage.ToolResult.Name != "slow" ||
		toolMessage.ToolResult.Content != context.DeadlineExceeded.Error() ||
		!toolMessage.ToolResult.IsError {
		t.Fatalf("ToolResult = %+v, want timeout error result", toolMessage.ToolResult)
	}
	assertTrace(t, state.Trace, []expectedTrace{
		{step: 0, action: TraceActionUserMessage, status: StatusRunning},
		{step: 1, action: TraceActionLLMRequest, status: StatusRunning},
		{step: 1, action: TraceActionLLMResponse, status: StatusRunning},
		{step: 1, action: TraceActionToolCall, status: StatusRunning, toolCallID: "call-timeout", toolName: "slow"},
		{step: 1, action: TraceActionToolTimeout, status: StatusRunning, toolCallID: "call-timeout", toolName: "slow", isError: true, wantErr: context.DeadlineExceeded},
		{step: 2, action: TraceActionLLMRequest, status: StatusRunning},
		{step: 2, action: TraceActionLLMResponse, status: StatusRunning},
		{step: 2, action: TraceActionFinal, status: StatusFinal},
	})
}

// TestRunWaitsForToolReturnBeforeContinuing 은 Tool 실행 수명과 Agent 상태 전이가 분리되지 않는지 확인한다.
func TestRunWaitsForToolReturnBeforeContinuing(t *testing.T) {
	executeErr := errors.New("controlled execution failed")
	tests := []struct {
		name            string
		result          runtimetool.Result
		executeErr      error
		waitForContext  bool
		toolTimeout     time.Duration
		cancelCaller    bool
		wantAction      TraceAction
		wantTraceErr    error
		wantResult      string
		wantResultError bool
	}{
		{
			name:       "success",
			result:     runtimetool.Result{Content: "controlled result"},
			wantAction: TraceActionToolResult,
			wantResult: "controlled result",
		},
		{
			name:            "execution error",
			executeErr:      executeErr,
			wantAction:      TraceActionToolError,
			wantTraceErr:    executeErr,
			wantResult:      executeErr.Error(),
			wantResultError: true,
		},
		{
			name:            "timeout",
			waitForContext:  true,
			toolTimeout:     10 * time.Millisecond,
			wantAction:      TraceActionToolTimeout,
			wantTraceErr:    context.DeadlineExceeded,
			wantResult:      context.DeadlineExceeded.Error(),
			wantResultError: true,
		},
		{
			name:            "caller cancellation",
			waitForContext:  true,
			toolTimeout:     time.Second,
			cancelCaller:    true,
			wantAction:      TraceActionToolError,
			wantTraceErr:    context.Canceled,
			wantResult:      context.Canceled.Error(),
			wantResultError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCall := message.ToolCall{ID: "call-controlled", Name: "controlled", Arguments: json.RawMessage(`{}`)}
			client := &stubClient{
				responses: []llm.ChatResponse{
					{Message: message.Assistant("using controlled tool", toolCall)},
					{Message: message.Assistant("final after controlled tool")},
				},
				callStart: make(chan int, 2),
			}
			controlled := newControlledTool(tt.result, tt.executeErr, tt.waitForContext)
			registry := runtimetool.NewRegistry()
			if err := registry.Register(controlled); err != nil {
				t.Fatalf("Register() error = %v", err)
			}
			agent, err := New(Options{
				Client:      client,
				Model:       "test-model",
				MaxSteps:    3,
				Tools:       registry,
				ToolTimeout: tt.toolTimeout,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			runCtx, cancelRun := context.WithCancel(context.Background())
			defer cancelRun()
			runDone := make(chan AgentState, 1)
			go func() {
				runDone <- agent.Run(runCtx, "use controlled tool")
			}()

			select {
			case <-controlled.started:
			case <-time.After(time.Second):
				t.Fatal("Tool Execute did not start")
			}
			if call := <-client.callStart; call != 1 {
				t.Fatalf("first model call = %d, want 1", call)
			}
			if tt.cancelCaller {
				cancelRun()
			}
			if tt.waitForContext {
				select {
				case <-controlled.contextDone:
				case <-time.After(time.Second):
					t.Fatal("Tool did not observe context cancellation")
				}
			}

			select {
			case call := <-client.callStart:
				close(controlled.release)
				t.Fatalf("model call %d started before Tool Execute returned", call)
			case <-runDone:
				close(controlled.release)
				t.Fatal("Agent returned before Tool Execute returned")
			case <-time.After(50 * time.Millisecond):
			}

			close(controlled.release)
			select {
			case <-controlled.returned:
			case <-time.After(time.Second):
				t.Fatal("Tool Execute did not return")
			}
			select {
			case call := <-client.callStart:
				if call != 2 {
					t.Fatalf("next model call = %d, want 2", call)
				}
			case <-time.After(time.Second):
				t.Fatal("next model call did not start after Tool Execute returned")
			}

			var state AgentState
			select {
			case state = <-runDone:
			case <-time.After(time.Second):
				t.Fatal("Agent did not return")
			}
			if state.Status != StatusFinal || state.FinalAnswer != "final after controlled tool" {
				t.Fatalf("state = %+v, want final result", state)
			}
			toolMessage := state.Messages[2]
			if toolMessage.ToolResult == nil || toolMessage.ToolResult.Content != tt.wantResult ||
				toolMessage.ToolResult.IsError != tt.wantResultError {
				t.Fatalf("ToolResult = %+v, want content=%q error=%v", toolMessage.ToolResult, tt.wantResult, tt.wantResultError)
			}
			assertTrace(t, state.Trace, []expectedTrace{
				{step: 0, action: TraceActionUserMessage, status: StatusRunning},
				{step: 1, action: TraceActionLLMRequest, status: StatusRunning},
				{step: 1, action: TraceActionLLMResponse, status: StatusRunning},
				{step: 1, action: TraceActionToolCall, status: StatusRunning, toolCallID: toolCall.ID, toolName: toolCall.Name},
				{step: 1, action: tt.wantAction, status: StatusRunning, toolCallID: toolCall.ID, toolName: toolCall.Name, isError: tt.wantResultError, wantErr: tt.wantTraceErr},
				{step: 2, action: TraceActionLLMRequest, status: StatusRunning},
				{step: 2, action: TraceActionLLMResponse, status: StatusRunning},
				{step: 2, action: TraceActionFinal, status: StatusFinal},
			})
		})
	}
}

// TestRunStopsAfterToolResultWhenMaxStepsReached 는 tool result 뒤 max step에 도달하면 다음 LLM 요청 없이 종료하는지 확인한다.
func TestRunStopsAfterToolResultWhenMaxStepsReached(t *testing.T) {
	toolCall := message.ToolCall{
		ID:        "call-1",
		Name:      "lookup",
		Arguments: json.RawMessage(`{"query":"runtime"}`),
	}
	client := &stubClient{
		responses: []llm.ChatResponse{
			{Message: message.Assistant("checking", toolCall)},
			{Message: message.Assistant("should not be used")},
		},
	}
	registry := runtimetool.NewRegistry()
	lookupTool := &stubTool{name: "lookup", result: runtimetool.Result{Content: "tool output"}}
	if err := registry.Register(lookupTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	agent, err := New(Options{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 1,
		Tools:    registry,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "use lookup")

	if state.Status != StatusMaxSteps {
		t.Fatalf("state Status = %q, want %q", state.Status, StatusMaxSteps)
	}
	if state.FinalAnswer != "" {
		t.Fatalf("FinalAnswer = %q, want empty", state.FinalAnswer)
	}
	if state.LastError != nil {
		t.Fatalf("LastError = %v, want nil", state.LastError)
	}
	if state.Step != 1 {
		t.Fatalf("Step = %d, want 1", state.Step)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
	if lookupTool.calls != 1 {
		t.Fatalf("tool calls = %d, want 1", lookupTool.calls)
	}
	if len(state.Messages) != 3 {
		t.Fatalf("len(state Messages) = %d, want user, assistant, tool", len(state.Messages))
	}
	toolMessage := state.Messages[2]
	if toolMessage.Role != message.RoleTool || toolMessage.ToolResult == nil || toolMessage.ToolResult.IsError {
		t.Fatalf("state Messages[2] = %+v, want successful tool result", toolMessage)
	}
	assertTrace(t, state.Trace, []expectedTrace{
		{step: 0, action: TraceActionUserMessage, status: StatusRunning},
		{step: 1, action: TraceActionLLMRequest, status: StatusRunning},
		{step: 1, action: TraceActionLLMResponse, status: StatusRunning},
		{step: 1, action: TraceActionToolCall, status: StatusRunning, toolCallID: "call-1", toolName: "lookup"},
		{step: 1, action: TraceActionToolResult, status: StatusRunning, toolCallID: "call-1", toolName: "lookup"},
		{step: 1, action: TraceActionMaxSteps, status: StatusMaxSteps},
	})
}

// TestRunStopsBeforeLLMWhenMaxStepsReached 는 max step 초과 시 provider 호출이 없는지 확인한다.
func TestRunStopsBeforeLLMWhenMaxStepsReached(t *testing.T) {
	client := &stubClient{
		response: llm.ChatResponse{
			Message: message.Assistant("should not be used"),
		},
	}
	agent, err := New(Options{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 0,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "hello runtime")

	if client.calls != 0 {
		t.Fatalf("client calls = %d, want 0", client.calls)
	}
	if state.Status != StatusMaxSteps {
		t.Fatalf("state Status = %q, want %q", state.Status, StatusMaxSteps)
	}
	if state.LastError != nil {
		t.Fatalf("LastError = %v, want nil", state.LastError)
	}
	if len(state.Messages) != 1 {
		t.Fatalf("len(state Messages) = %d, want 1", len(state.Messages))
	}
	if state.Messages[0].Role != message.RoleUser || state.Messages[0].Text != "hello runtime" {
		t.Fatalf("state user message = %+v, want role=user text=hello runtime", state.Messages[0])
	}
	assertTrace(t, state.Trace, []expectedTrace{
		{step: 0, action: TraceActionUserMessage, status: StatusRunning},
		{step: 0, action: TraceActionMaxSteps, status: StatusMaxSteps},
	})
}

// TestRunStoresLLMErrorWithoutAssistantMessage 는 LLM 오류가 상태에 남고 assistant가 누적되지 않는지 확인한다.
func TestRunStoresLLMErrorWithoutAssistantMessage(t *testing.T) {
	llmErr := errors.New("llm failed")
	client := &stubClient{err: llmErr}
	agent, err := New(Options{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 3,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "hello runtime")

	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
	if state.Status != StatusError {
		t.Fatalf("state Status = %q, want %q", state.Status, StatusError)
	}
	if !errors.Is(state.LastError, llmErr) {
		t.Fatalf("LastError = %v, want %v", state.LastError, llmErr)
	}
	if len(state.Messages) != 1 {
		t.Fatalf("len(state Messages) = %d, want 1", len(state.Messages))
	}
	if state.Messages[0].Role != message.RoleUser || state.Messages[0].Text != "hello runtime" {
		t.Fatalf("state user message = %+v, want role=user text=hello runtime", state.Messages[0])
	}
	assertTrace(t, state.Trace, []expectedTrace{
		{step: 0, action: TraceActionUserMessage, status: StatusRunning},
		{step: 1, action: TraceActionLLMRequest, status: StatusRunning},
		{step: 1, action: TraceActionLLMError, status: StatusError, wantErr: llmErr},
	})
}

func TestRunStopsBeforeExceedingDefaultToolCallLimit(t *testing.T) {
	calls := make([]message.ToolCall, defaultMaxToolCalls+1)
	for i := range calls {
		calls[i] = message.ToolCall{
			ID:        fmt.Sprintf("call-%d", i+1),
			Name:      "lookup",
			Arguments: json.RawMessage(`{}`),
		}
	}
	client := &stubClient{response: llm.ChatResponse{Message: message.Assistant("many calls", calls...)}}
	lookupTool := &stubTool{name: "lookup", result: runtimetool.Result{Content: "ok"}}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(lookupTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	agent, err := New(Options{Client: client, MaxSteps: 2, Tools: registry})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "use many tools")

	if state.Status != StatusError || client.calls != 1 || lookupTool.calls != defaultMaxToolCalls {
		t.Fatalf("status/client calls/tool calls = %q/%d/%d, want error/1/%d", state.Status, client.calls, lookupTool.calls, defaultMaxToolCalls)
	}
	var limitErr *RunnerError
	if !errors.As(state.LastError, &limitErr) || limitErr.Kind != RunnerErrorKindExecutionLimit ||
		limitErr.Limit != limitMaxToolCalls || limitErr.Current != defaultMaxToolCalls+1 || limitErr.Maximum != defaultMaxToolCalls {
		t.Fatalf("LastError = %+v, want max tool calls execution limit", state.LastError)
	}
	lastTrace := state.Trace[len(state.Trace)-1]
	if lastTrace.Action != TraceActionExecutionLimit || lastTrace.Status != StatusError || !errors.Is(lastTrace.Error, state.LastError) {
		t.Fatalf("last trace = %+v, want execution limit error", lastTrace)
	}
}

func TestRunAppliesToolCallLimitAcrossModelSteps(t *testing.T) {
	toolCall := func(id string) message.ToolCall {
		return message.ToolCall{ID: id, Name: "lookup", Arguments: json.RawMessage(`{}`)}
	}
	client := &stubClient{responses: []llm.ChatResponse{
		{Message: message.Assistant("first", toolCall("call-1"))},
		{Message: message.Assistant("second", toolCall("call-2"))},
		{Message: message.Assistant("third", toolCall("call-3"))},
	}}
	lookupTool := &stubTool{name: "lookup", result: runtimetool.Result{Content: "ok"}}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(lookupTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	agent, err := New(Options{Client: client, MaxSteps: 4, Tools: registry, MaxToolCalls: 2})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	state := agent.Run(context.Background(), "use tools across steps")

	if state.Status != StatusError || client.calls != 3 || lookupTool.calls != 2 {
		t.Fatalf("status/client calls/tool calls = %q/%d/%d, want error/3/2", state.Status, client.calls, lookupTool.calls)
	}
	var limitErr *RunnerError
	if !errors.As(state.LastError, &limitErr) || limitErr.Limit != limitMaxToolCalls || limitErr.Current != 3 || limitErr.Maximum != 2 {
		t.Fatalf("LastError = %+v, want cumulative max tool calls limit", state.LastError)
	}
}

func TestRunLimitsToolResultBytes(t *testing.T) {
	limit := runtimetool.DefaultMaxResultBytes
	tests := []struct {
		name        string
		content     string
		wantError   bool
		wantCurrent int
	}{
		{name: "at limit", content: strings.Repeat("1", limit)},
		{name: "over limit", content: strings.Repeat("1", limit+1), wantError: true, wantCurrent: limit + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := message.ToolCall{ID: "call-1", Name: "lookup", Arguments: json.RawMessage(`{}`)}
			client := &stubClient{responses: []llm.ChatResponse{
				{Message: message.Assistant("lookup", call)},
				{Message: message.Assistant("done")},
			}}
			lookupTool := &stubTool{name: "lookup", result: runtimetool.Result{Content: tt.content}}
			registry := runtimetool.NewRegistry()
			if err := registry.Register(lookupTool); err != nil {
				t.Fatalf("Register() error = %v", err)
			}
			agent, err := New(Options{Client: client, MaxSteps: 3, Tools: registry})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			state := agent.Run(context.Background(), "use lookup")

			if state.Status != StatusFinal || state.LastError != nil || client.calls != 2 {
				t.Fatalf("state/client calls = %+v/%d, want final with two calls", state, client.calls)
			}
			result := client.requests[1].Messages[2].ToolResult
			if result == nil || result.IsError != tt.wantError {
				t.Fatalf("ToolResult = %+v, want error=%v", result, tt.wantError)
			}
			if !tt.wantError {
				if result.Content != tt.content {
					t.Fatalf("ToolResult.Content = %q, want %q", result.Content, tt.content)
				}
				return
			}
			if strings.Contains(result.Content, tt.content) {
				t.Fatalf("ToolResult.Content contains oversized payload: %q", result.Content)
			}
			var limitErr *RunnerError
			traceErr := state.Trace[4].Error
			if !errors.As(traceErr, &limitErr) || limitErr.Limit != limitToolResultBytes ||
				limitErr.Current != tt.wantCurrent || limitErr.Maximum != limit {
				t.Fatalf("tool trace error = %+v, want result byte limit", traceErr)
			}
		})
	}
}

func TestRunClassifiesCallerDeadlineAsExecutionLimit(t *testing.T) {
	client := &contextStubClient{}
	agent, err := New(Options{Client: client, MaxSteps: 1})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	state := agent.Run(ctx, "wait")

	var limitErr *RunnerError
	if state.Status != StatusError || !errors.As(state.LastError, &limitErr) ||
		limitErr.Limit != limitRunDeadline || !errors.Is(state.LastError, context.DeadlineExceeded) {
		t.Fatalf("state = %+v, want run deadline execution limit", state)
	}
	lastTrace := state.Trace[len(state.Trace)-1]
	if lastTrace.Action != TraceActionExecutionLimit || lastTrace.Status != StatusError {
		t.Fatalf("last trace = %+v, want execution limit", lastTrace)
	}
}

func TestRunClassifiesCallerDeadlineDuringToolAsExecutionLimit(t *testing.T) {
	call := message.ToolCall{ID: "call-1", Name: "slow", Arguments: json.RawMessage(`{}`)}
	client := &stubClient{response: llm.ChatResponse{Message: message.Assistant("wait", call)}}
	slowTool := &stubTool{name: "slow", waitForCtx: true}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(slowTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	agent, err := New(Options{Client: client, MaxSteps: 2, Tools: registry, ToolTimeout: time.Second})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	state := agent.Run(ctx, "use slow tool")

	var limitErr *RunnerError
	if state.Status != StatusError || slowTool.calls != 1 || !errors.As(state.LastError, &limitErr) ||
		limitErr.Limit != limitRunDeadline || !errors.Is(state.LastError, context.DeadlineExceeded) {
		t.Fatalf("state/tool calls = %+v/%d, want run deadline execution limit", state, slowTool.calls)
	}
	lastTrace := state.Trace[len(state.Trace)-1]
	if lastTrace.Action != TraceActionExecutionLimit || lastTrace.Status != StatusError {
		t.Fatalf("last trace = %+v, want execution limit", lastTrace)
	}
}

func TestNewRejectsNegativeToolLimits(t *testing.T) {
	client := &stubClient{}
	tests := []struct {
		name string
		opts Options
	}{
		{name: "tool calls", opts: Options{Client: client, MaxToolCalls: -1}},
		{name: "tool result bytes", opts: Options{Client: client, MaxToolResultBytes: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.opts); err == nil {
				t.Fatal("New() error = nil, want negative limit error")
			}
		})
	}
}
