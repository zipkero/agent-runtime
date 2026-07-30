package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	runtimetool "github.com/zipkero/agent-runtime/internal/tool"
)

// TestRunnerAppliesMiddlewareInRegistrationOrderOnEveryModelCall 은 pre/post hook이 매 모델 호출마다 등록 순서대로 실행되고,
// 앞 hook의 변경이 다음 hook과 실제 provider 요청에까지 이어지는지 확인한다.
func TestRunnerAppliesMiddlewareInRegistrationOrderOnEveryModelCall(t *testing.T) {
	client := &stubClient{responses: []llm.ChatResponse{
		{Message: message.Assistant("lookup pending", message.ToolCall{
			ID:        "call-1",
			Name:      "unresolved",
			Arguments: json.RawMessage(`{"query":"runtime"}`),
		})},
		{Message: message.Assistant("answer")},
	}}
	lookup := &stubTool{
		name:        "lookup",
		description: "Lookup runtime data",
		schema: message.ToolSchema{
			Name:        "lookup",
			Description: "Lookup runtime data",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		result: runtimetool.Result{Content: "tool output"},
	}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(lookup); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	var order []string
	middleware := []ModelMiddleware{
		{
			Name: "first",
			PreModel: func(_ context.Context, req llm.ChatRequest) (llm.ChatRequest, error) {
				order = append(order, "pre:first")
				req.Messages = append(req.Messages, message.System("first"))
				return req, nil
			},
			PostModel: func(_ context.Context, _ llm.ChatRequest, resp llm.ChatResponse) (llm.ChatResponse, error) {
				order = append(order, "post:first")
				if len(resp.Message.ToolCalls) > 0 {
					resp.Message.ToolCalls[0].Name = "lookup"
				} else {
					resp.Message.Text += " from first"
				}
				return resp, nil
			},
		},
		{
			Name: "second",
			PreModel: func(_ context.Context, req llm.ChatRequest) (llm.ChatRequest, error) {
				order = append(order, "pre:second")
				if got := req.Messages[len(req.Messages)-1].Text; got != "first" {
					t.Fatalf("second pre-model last message = %q, want first hook change", got)
				}
				req.Messages[len(req.Messages)-1].Text = "second"
				return req, nil
			},
			PostModel: func(_ context.Context, _ llm.ChatRequest, resp llm.ChatResponse) (llm.ChatResponse, error) {
				order = append(order, "post:second")
				if len(resp.Message.ToolCalls) > 0 {
					if got := resp.Message.ToolCalls[0].Name; got != "lookup" {
						t.Fatalf("second post-model tool name = %q, want first hook change", got)
					}
				} else {
					if got := resp.Message.Text; got != "answer from first" {
						t.Fatalf("second post-model text = %q, want first hook change", got)
					}
					resp.Message.Text += " and second"
				}
				return resp, nil
			},
		},
	}

	runner, err := NewRunner(RunnerOptions{
		Client:     client,
		MaxSteps:   3,
		Tools:      registry,
		Middleware: middleware,
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result := runner.Run(context.Background(), "use lookup")

	wantOrder := []string{
		"pre:first", "pre:second", "post:first", "post:second",
		"pre:first", "pre:second", "post:first", "post:second",
	}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("middleware order = %v, want %v", order, wantOrder)
	}
	if result.State.Status != StatusFinal || result.State.FinalAnswer != "answer from first and second" {
		t.Fatalf("State = %+v, want middleware-modified final answer", result.State)
	}
	if client.calls != 2 || lookup.calls != 1 {
		t.Fatalf("client/tool calls = %d/%d, want 2/1", client.calls, lookup.calls)
	}
	for i, req := range client.requests {
		if got := req.Messages[len(req.Messages)-1].Text; got != "second" {
			t.Fatalf("request[%d] last message = %q, want pre-model change", i, got)
		}
	}
}

// TestRunnerAppliesModelTimeoutOnlyToProviderCall 은 ModelTimeout이 provider 호출 context에만 붙고 middleware 실행에는 걸리지 않는지 확인한다.
func TestRunnerAppliesModelTimeoutOnlyToProviderCall(t *testing.T) {
	client := &deadlineStubClient{responses: []llm.ChatResponse{{Message: message.Assistant("answer")}}}
	middleware := ModelMiddleware{
		Name: "context-observer",
		PreModel: func(ctx context.Context, req llm.ChatRequest) (llm.ChatRequest, error) {
			if _, ok := ctx.Deadline(); ok {
				t.Fatal("pre-model context has provider deadline")
			}
			return req, nil
		},
		PostModel: func(ctx context.Context, _ llm.ChatRequest, resp llm.ChatResponse) (llm.ChatResponse, error) {
			if _, ok := ctx.Deadline(); ok {
				t.Fatal("post-model context has provider deadline")
			}
			return resp, nil
		},
	}
	runner, err := NewRunner(RunnerOptions{
		Client:       client,
		MaxSteps:     1,
		ModelTimeout: 100 * time.Millisecond,
		Middleware:   []ModelMiddleware{middleware},
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result := runner.Run(context.Background(), "hello")

	if result.State.Status != StatusFinal || result.State.FinalAnswer != "answer" {
		t.Fatalf("State = %+v, want final answer", result.State)
	}
	if len(client.deadlines) != 1 {
		t.Fatalf("len(deadlines) = %d, want provider deadline", len(client.deadlines))
	}
}

// TestPreModelChangesDoNotAliasAgentOrRegistryState 는 pre-model hook이 받은 요청을 직접 수정해도
// Agent 대화 상태와 registry schema가 오염되지 않는지 확인한다.
func TestPreModelChangesDoNotAliasAgentOrRegistryState(t *testing.T) {
	originalArguments := json.RawMessage(`{"query":"runtime"}`)
	client := &stubClient{responses: []llm.ChatResponse{
		{Message: message.Assistant("checking", message.ToolCall{
			ID:        "call-1",
			Name:      "lookup",
			Arguments: originalArguments,
		})},
		{Message: message.Assistant("done")},
	}}
	lookup := &stubTool{
		name:        "lookup",
		description: "Lookup runtime data",
		schema: message.ToolSchema{
			Name:        "lookup",
			Description: "Lookup runtime data",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		result: runtimetool.Result{Content: "tool output"},
	}
	registry := runtimetool.NewRegistry()
	if err := registry.Register(lookup); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	middleware := ModelMiddleware{
		Name: "mutator",
		PreModel: func(_ context.Context, req llm.ChatRequest) (llm.ChatRequest, error) {
			req.Messages[0].Text = "changed user"
			req.Tools[0].InputSchema[0] = '['
			if len(req.Messages) > 1 {
				req.Messages[1].ToolCalls[0].Arguments[0] = '['
				req.Messages[2].ToolResult.Content = "changed result"
			}
			return req, nil
		},
		PostModel: func(_ context.Context, _ llm.ChatRequest, resp llm.ChatResponse) (llm.ChatResponse, error) {
			if len(resp.Message.ToolCalls) > 0 {
				resp.Message.ToolCalls[0].Arguments[0] = '['
			}
			return resp, nil
		},
	}
	runner, err := NewRunner(RunnerOptions{
		Client:     client,
		MaxSteps:   3,
		Tools:      registry,
		Middleware: []ModelMiddleware{middleware},
	})
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	result := runner.Run(context.Background(), "use lookup")

	if result.State.Status != StatusFinal {
		t.Fatalf("State = %+v, want final", result.State)
	}
	if got := result.State.Messages[0].Text; got != "use lookup" {
		t.Fatalf("agent user message = %q, want original", got)
	}
	if got := string(result.State.Messages[1].ToolCalls[0].Arguments); got != `["query":"runtime"}` {
		t.Fatalf("agent assistant arguments = %q, want post-model change only", got)
	}
	if got := result.State.Messages[2].ToolResult.Content; got != "tool output" {
		t.Fatalf("agent tool result = %q, want original", got)
	}
	if got := string(registry.Schemas()[0].InputSchema); got != `{"type":"object"}` {
		t.Fatalf("registry schema = %q, want original", got)
	}
}

// TestRunnerStopsAtMiddlewareError 는 middleware 실패가 실패한 단계와 이름을 보존한 오류로 끊기고,
// 뒤 middleware와 tool을 실행하지 않으며 응답도 누적하지 않는지 확인한다.
func TestRunnerStopsAtMiddlewareError(t *testing.T) {
	tests := []struct {
		name            string
		middleware      func(error, *int) []ModelMiddleware
		wantStage       MiddlewareStage
		wantMiddleware  string
		wantCalls       int
		wantToolCalls   int
		wantLLMRequests int
	}{
		{
			name: "pre-model",
			middleware: func(sentinel error, laterCalls *int) []ModelMiddleware {
				return []ModelMiddleware{
					{Name: "pre-failure", PreModel: func(context.Context, llm.ChatRequest) (llm.ChatRequest, error) {
						return llm.ChatRequest{}, sentinel
					}},
					{Name: "later", PreModel: func(_ context.Context, req llm.ChatRequest) (llm.ChatRequest, error) {
						*laterCalls++
						return req, nil
					}},
				}
			},
			wantStage:      MiddlewareStagePreModel,
			wantMiddleware: "pre-failure",
		},
		{
			name: "post-model",
			middleware: func(sentinel error, laterCalls *int) []ModelMiddleware {
				return []ModelMiddleware{
					{Name: "post-failure", PostModel: func(context.Context, llm.ChatRequest, llm.ChatResponse) (llm.ChatResponse, error) {
						return llm.ChatResponse{}, sentinel
					}},
					{Name: "later", PostModel: func(_ context.Context, _ llm.ChatRequest, resp llm.ChatResponse) (llm.ChatResponse, error) {
						*laterCalls++
						return resp, nil
					}},
				}
			},
			wantStage:       MiddlewareStagePostModel,
			wantMiddleware:  "post-failure",
			wantCalls:       1,
			wantLLMRequests: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sentinel := errors.New("middleware failed")
			laterCalls := 0
			client := &stubClient{response: llm.ChatResponse{Message: message.Assistant("checking", message.ToolCall{
				ID:        "call-1",
				Name:      "lookup",
				Arguments: json.RawMessage(`{}`),
			})}}
			lookup := &stubTool{
				name:        "lookup",
				description: "Lookup runtime data",
				schema: message.ToolSchema{
					Name:        "lookup",
					Description: "Lookup runtime data",
					InputSchema: json.RawMessage(`{"type":"object"}`),
				},
			}
			registry := runtimetool.NewRegistry()
			if err := registry.Register(lookup); err != nil {
				t.Fatalf("Register() error = %v", err)
			}
			runner, err := NewRunner(RunnerOptions{
				Client:     client,
				MaxSteps:   3,
				Tools:      registry,
				Middleware: tt.middleware(sentinel, &laterCalls),
			})
			if err != nil {
				t.Fatalf("NewRunner() error = %v", err)
			}

			result := runner.Run(context.Background(), "use lookup")

			if result.State.Status != StatusError || !errors.Is(result.State.LastError, sentinel) {
				t.Fatalf("State = %+v, want middleware error", result.State)
			}
			var runnerErr *RunnerError
			if !errors.As(result.State.LastError, &runnerErr) {
				t.Fatalf("LastError = %T, want *RunnerError", result.State.LastError)
			}
			if runnerErr.Kind != RunnerErrorKindMiddleware || runnerErr.Stage != tt.wantStage || runnerErr.Middleware != tt.wantMiddleware {
				t.Fatalf("RunnerError = %+v, want middleware/%s/%s", runnerErr, tt.wantStage, tt.wantMiddleware)
			}
			if !IsRunnerErrorKind(result.State.LastError, RunnerErrorKindMiddleware) {
				t.Fatal("IsRunnerErrorKind() = false, want true")
			}
			if client.calls != tt.wantCalls || lookup.calls != tt.wantToolCalls || laterCalls != 0 {
				t.Fatalf("client/tool/later calls = %d/%d/%d, want %d/%d/0", client.calls, lookup.calls, laterCalls, tt.wantCalls, tt.wantToolCalls)
			}
			if len(result.State.Messages) != 1 {
				t.Fatalf("len(Messages) = %d, want response not accumulated", len(result.State.Messages))
			}
			llmRequests := 0
			for _, event := range result.State.Trace {
				if event.Action == TraceActionLLMRequest {
					llmRequests++
				}
			}
			if llmRequests != tt.wantLLMRequests {
				t.Fatalf("LLM request traces = %d, want %d", llmRequests, tt.wantLLMRequests)
			}
			lastTrace := result.State.Trace[len(result.State.Trace)-1]
			if lastTrace.Action != TraceActionMiddlewareError || !errors.Is(lastTrace.Error, sentinel) {
				t.Fatalf("last trace = %+v, want middleware error trace", lastTrace)
			}
		})
	}
}

// TestNewRunnerValidatesMiddleware 는 이름 누락·공백·중복과 hook 없는 middleware를 실행 전에 거절하는지 확인한다.
func TestNewRunnerValidatesMiddleware(t *testing.T) {
	preModel := func(_ context.Context, req llm.ChatRequest) (llm.ChatRequest, error) {
		return req, nil
	}
	tests := []struct {
		name       string
		middleware []ModelMiddleware
	}{
		{
			name:       "missing name",
			middleware: []ModelMiddleware{{PreModel: preModel}},
		},
		{
			name:       "whitespace name",
			middleware: []ModelMiddleware{{Name: "   ", PreModel: preModel}},
		},
		{
			name:       "surrounding whitespace",
			middleware: []ModelMiddleware{{Name: " policy ", PreModel: preModel}},
		},
		{
			name:       "missing hooks",
			middleware: []ModelMiddleware{{Name: "empty"}},
		},
		{
			name: "duplicate name",
			middleware: []ModelMiddleware{
				{Name: "policy", PreModel: preModel},
				{Name: "policy", PreModel: preModel},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRunner(RunnerOptions{
				Client:     &stubClient{},
				MaxSteps:   1,
				Middleware: tt.middleware,
			})
			if err == nil {
				t.Fatal("NewRunner() error = nil, want invalid middleware error")
			}
		})
	}
}
