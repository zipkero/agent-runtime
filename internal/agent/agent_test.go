package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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
}

func (c *stubClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls++
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
	name        string
	description string
	schema      message.ToolSchema
	result      runtimetool.Result
	args        json.RawMessage
	calls       int
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
	return nil
}

func (t *stubTool) Execute(_ context.Context, args json.RawMessage) (runtimetool.Result, error) {
	t.calls++
	t.args = append(json.RawMessage(nil), args...)
	return t.result, nil
}

type expectedTrace struct {
	step    int
	action  TraceAction
	status  Status
	wantErr error
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
