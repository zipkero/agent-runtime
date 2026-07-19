package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	runtimetool "github.com/zipkero/agent-runtime/internal/tool"
)

func TestRunnerExecutesToolLoopAndReturnsFinalState(t *testing.T) {
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
	registry := runtimetool.NewRegistry()
	if err := registry.Register(lookupTool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	runner, err := NewRunner(RunnerOptions{
		Client:   client,
		Model:    "test-model",
		MaxSteps: 3,
		Tools:    registry,
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result := runner.Run(context.Background(), "use lookup")

	if result.State.Status != StatusFinal || result.State.FinalAnswer != "final answer" {
		t.Fatalf("State = %+v, want final answer", result.State)
	}
	if result.StructuredOutput != nil {
		t.Fatalf("StructuredOutput = %s, want nil without schema", result.StructuredOutput)
	}
	if result.State.Step != 2 || client.calls != 2 || lookupTool.calls != 1 {
		t.Fatalf("steps/client calls/tool calls = %d/%d/%d, want 2/2/1", result.State.Step, client.calls, lookupTool.calls)
	}
	for i, req := range client.requests {
		if req.Model != "test-model" {
			t.Fatalf("request[%d].Model = %q, want test-model", i, req.Model)
		}
		if len(req.Tools) != 1 || req.Tools[0].Name != "lookup" {
			t.Fatalf("request[%d].Tools = %+v, want lookup schema", i, req.Tools)
		}
	}
	if len(client.requests[1].Messages) != 3 {
		t.Fatalf("second request len(Messages) = %d, want user, assistant, tool", len(client.requests[1].Messages))
	}
	toolMessage := client.requests[1].Messages[2]
	if toolMessage.Role != message.RoleTool || toolMessage.ToolResult == nil {
		t.Fatalf("second request tool message = %+v, want tool result", toolMessage)
	}
	if toolMessage.ToolResult.ToolCallID != "call-1" || toolMessage.ToolResult.Content != "tool output" {
		t.Fatalf("second request ToolResult = %+v, want lookup output", toolMessage.ToolResult)
	}
}

func TestRunnerAppliesIndependentTimeoutToEveryModelCall(t *testing.T) {
	client := &deadlineStubClient{
		responses: []llm.ChatResponse{
			{Message: message.Assistant("waiting", message.ToolCall{
				ID:        "call-1",
				Name:      "delay",
				Arguments: json.RawMessage(`{}`),
			})},
			{Message: message.Assistant("done")},
		},
	}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(delayTool{delay: 50 * time.Millisecond}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	runner, err := NewRunner(RunnerOptions{
		Client:       client,
		Model:        "test-model",
		MaxSteps:     3,
		ModelTimeout: 100 * time.Millisecond,
		Tools:        registry,
		ToolTimeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result := runner.Run(context.Background(), "wait and finish")

	if result.State.Status != StatusFinal || result.State.FinalAnswer != "done" {
		t.Fatalf("State = %+v, want final done", result.State)
	}
	if len(client.deadlines) != 2 {
		t.Fatalf("len(deadlines) = %d, want 2", len(client.deadlines))
	}
	for i, remaining := range client.deadlines {
		if remaining < 80*time.Millisecond || remaining > 100*time.Millisecond {
			t.Fatalf("deadline[%d] remaining = %v, want independently near 100ms", i, remaining)
		}
	}
}

func TestRunnerPreservesMaxStepsAndLLMErrorStates(t *testing.T) {
	t.Run("max steps", func(t *testing.T) {
		client := &stubClient{response: llm.ChatResponse{Message: message.Assistant("answer")}}
		runner, err := NewRunner(RunnerOptions{Client: client, MaxSteps: 0})
		if err != nil {
			t.Fatalf("NewRunner() error = %v", err)
		}

		result := runner.Run(context.Background(), "hello")

		if result.State.Status != StatusMaxSteps || result.State.Step != 0 || client.calls != 0 {
			t.Fatalf("State/client calls = %+v/%d, want max steps before model call", result.State, client.calls)
		}
	})

	t.Run("llm error", func(t *testing.T) {
		sentinelErr := errors.New("provider unavailable")
		client := &stubClient{err: sentinelErr}
		runner, err := NewRunner(RunnerOptions{Client: client, MaxSteps: 1})
		if err != nil {
			t.Fatalf("NewRunner() error = %v", err)
		}

		result := runner.Run(context.Background(), "hello")

		if result.State.Status != StatusError || !errors.Is(result.State.LastError, sentinelErr) {
			t.Fatalf("State = %+v, want LLM error", result.State)
		}
	})
}

func TestRunnerPreservesCallerCancellation(t *testing.T) {
	client := &contextStubClient{}
	runner, err := NewRunner(RunnerOptions{
		Client:       client,
		MaxSteps:     1,
		ModelTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := runner.Run(ctx, "hello")

	if result.State.Status != StatusError || !errors.Is(result.State.LastError, context.Canceled) {
		t.Fatalf("State = %+v, want caller cancellation", result.State)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1 canceled call", client.calls)
	}
}

func TestNewRunnerRequiresClient(t *testing.T) {
	tests := []struct {
		name         string
		modelTimeout time.Duration
	}{
		{name: "without model timeout"},
		{name: "with model timeout", modelTimeout: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRunner(RunnerOptions{MaxSteps: 1, ModelTimeout: tt.modelTimeout})
			if err == nil {
				t.Fatal("NewRunner() error = nil, want required client error")
			}
		})
	}
}

func TestNewRunnerValidatesTimeoutAndToolLimitBoundaries(t *testing.T) {
	client := &stubClient{}
	tests := []struct {
		name               string
		opts               RunnerOptions
		wantError          bool
		wantModelTimeout   time.Duration
		wantToolTimeout    time.Duration
		wantMaxToolCalls   int
		wantMaxResultBytes int
	}{
		{
			name: "positive values",
			opts: RunnerOptions{
				Client:             client,
				ModelTimeout:       time.Second,
				ToolTimeout:        2 * time.Second,
				MaxToolCalls:       1,
				MaxToolResultBytes: 1,
			},
			wantModelTimeout:   time.Second,
			wantToolTimeout:    2 * time.Second,
			wantMaxToolCalls:   1,
			wantMaxResultBytes: 1,
		},
		{
			name:               "zero defaults",
			opts:               RunnerOptions{Client: client},
			wantToolTimeout:    defaultToolTimeout,
			wantMaxToolCalls:   defaultMaxToolCalls,
			wantMaxResultBytes: runtimetool.DefaultMaxResultBytes,
		},
		{name: "negative model timeout", opts: RunnerOptions{Client: client, ModelTimeout: -time.Nanosecond}, wantError: true},
		{name: "negative tool timeout", opts: RunnerOptions{Client: client, ToolTimeout: -time.Nanosecond}, wantError: true},
		{name: "negative tool calls", opts: RunnerOptions{Client: client, MaxToolCalls: -1}, wantError: true},
		{name: "negative tool result bytes", opts: RunnerOptions{Client: client, MaxToolResultBytes: -1}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner, err := NewRunner(tt.opts)
			if tt.wantError {
				if err == nil {
					t.Fatal("NewRunner() error = nil, want invalid option error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewRunner() error = %v", err)
			}
			if runner.agent.modelTimeout != tt.wantModelTimeout ||
				runner.agent.toolTimeout != tt.wantToolTimeout ||
				runner.agent.maxToolCalls != tt.wantMaxToolCalls ||
				runner.agent.maxToolResultBytes != tt.wantMaxResultBytes {
				t.Fatalf(
					"limits = %s/%s/%d/%d, want %s/%s/%d/%d",
					runner.agent.modelTimeout,
					runner.agent.toolTimeout,
					runner.agent.maxToolCalls,
					runner.agent.maxToolResultBytes,
					tt.wantModelTimeout,
					tt.wantToolTimeout,
					tt.wantMaxToolCalls,
					tt.wantMaxResultBytes,
				)
			}
		})
	}
}

type deadlineStubClient struct {
	responses []llm.ChatResponse
	deadlines []time.Duration
}

func (c *deadlineStubClient) Chat(ctx context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return llm.ChatResponse{}, errors.New("model context has no deadline")
	}
	c.deadlines = append(c.deadlines, time.Until(deadline))
	response := c.responses[0]
	c.responses = c.responses[1:]
	return response, nil
}

type contextStubClient struct {
	calls int
}

func (c *contextStubClient) Chat(ctx context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls++
	<-ctx.Done()
	return llm.ChatResponse{}, ctx.Err()
}

type delayTool struct {
	delay time.Duration
}

func (delayTool) Name() string {
	return "delay"
}

func (delayTool) Description() string {
	return "Delay before returning."
}

func (delayTool) Schema() message.ToolSchema {
	return message.ToolSchema{
		Name:        "delay",
		Description: "Delay before returning.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (delayTool) Validate(json.RawMessage) error {
	return nil
}

func (t delayTool) Execute(ctx context.Context, _ json.RawMessage) (runtimetool.Result, error) {
	select {
	case <-time.After(t.delay):
		return runtimetool.Result{Content: "delayed"}, nil
	case <-ctx.Done():
		return runtimetool.Result{}, ctx.Err()
	}
}
