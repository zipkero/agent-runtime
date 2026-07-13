package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
)

// MiddlewareStage 는 model 호출에서 middleware가 실행되는 경계를 식별한다.
type MiddlewareStage string

const (
	MiddlewareStagePreModel  MiddlewareStage = "pre_model"
	MiddlewareStagePostModel MiddlewareStage = "post_model"
)

// RunnerErrorKind 는 Runner 실행 경계에서 분류하는 오류 종류다.
type RunnerErrorKind string

const (
	RunnerErrorKindMiddleware RunnerErrorKind = "middleware"
)

// RunnerError 는 Runner 고유 실패 분류와 원인을 함께 보존한다.
type RunnerError struct {
	Kind       RunnerErrorKind
	Stage      MiddlewareStage
	Middleware string
	Err        error
}

func (e *RunnerError) Error() string {
	return fmt.Sprintf("agent runner %s %s middleware %q: %v", e.Kind, e.Stage, e.Middleware, e.Err)
}

func (e *RunnerError) Unwrap() error {
	return e.Err
}

// IsRunnerErrorKind 는 오류 chain에 지정한 Runner 오류 종류가 있는지 확인한다.
func IsRunnerErrorKind(err error, kind RunnerErrorKind) bool {
	var runnerErr *RunnerError
	return errors.As(err, &runnerErr) && runnerErr.Kind == kind
}

type PreModelHook func(context.Context, llm.ChatRequest) (llm.ChatRequest, error)

type PostModelHook func(context.Context, llm.ChatRequest, llm.ChatResponse) (llm.ChatResponse, error)

// ModelMiddleware 는 model 요청과 정규화된 응답을 순차 처리하는 선택적 hook 묶음이다.
type ModelMiddleware struct {
	Name      string
	PreModel  PreModelHook
	PostModel PostModelHook
}

type middlewareClient struct {
	client     llm.LLMClient
	middleware []ModelMiddleware
}

func newMiddlewareClient(client llm.LLMClient, middleware []ModelMiddleware) (llm.LLMClient, error) {
	for i, item := range middleware {
		if item.Name == "" {
			return nil, fmt.Errorf("agent runner middleware[%d] name is required", i)
		}
		if item.PreModel == nil && item.PostModel == nil {
			return nil, fmt.Errorf("agent runner middleware %q requires a pre-model or post-model hook", item.Name)
		}
	}

	return &middlewareClient{
		client:     client,
		middleware: append([]ModelMiddleware(nil), middleware...),
	}, nil
}

func (c *middlewareClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	processedRequest := cloneChatRequest(req)
	for _, item := range c.middleware {
		if item.PreModel == nil {
			continue
		}

		next, err := item.PreModel(ctx, cloneChatRequest(processedRequest))
		if err != nil {
			return llm.ChatResponse{}, middlewareError(MiddlewareStagePreModel, item.Name, err)
		}
		processedRequest = cloneChatRequest(next)
	}

	response, err := c.client.Chat(ctx, cloneChatRequest(processedRequest))
	if err != nil {
		return llm.ChatResponse{}, err
	}

	processedResponse := cloneChatResponse(response)
	for _, item := range c.middleware {
		if item.PostModel == nil {
			continue
		}

		next, err := item.PostModel(ctx, cloneChatRequest(processedRequest), cloneChatResponse(processedResponse))
		if err != nil {
			return llm.ChatResponse{}, middlewareError(MiddlewareStagePostModel, item.Name, err)
		}
		processedResponse = cloneChatResponse(next)
	}

	return processedResponse, nil
}

func middlewareError(stage MiddlewareStage, name string, err error) error {
	return &RunnerError{
		Kind:       RunnerErrorKindMiddleware,
		Stage:      stage,
		Middleware: name,
		Err:        err,
	}
}

func cloneChatRequest(req llm.ChatRequest) llm.ChatRequest {
	cloned := req
	cloned.Messages = cloneMessages(req.Messages)
	cloned.Tools = make([]message.ToolSchema, len(req.Tools))
	for i, schema := range req.Tools {
		cloned.Tools[i] = schema
		cloned.Tools[i].InputSchema = cloneRawMessage(schema.InputSchema)
	}
	return cloned
}

func cloneChatResponse(response llm.ChatResponse) llm.ChatResponse {
	cloned := response
	cloned.Message = cloneMessage(response.Message)
	return cloned
}

func cloneMessages(messages []message.Message) []message.Message {
	cloned := make([]message.Message, len(messages))
	for i, item := range messages {
		cloned[i] = cloneMessage(item)
	}
	return cloned
}

func cloneMessage(item message.Message) message.Message {
	cloned := item
	cloned.ToolCalls = make([]message.ToolCall, len(item.ToolCalls))
	for i, call := range item.ToolCalls {
		cloned.ToolCalls[i] = call
		cloned.ToolCalls[i].Arguments = cloneRawMessage(call.Arguments)
	}
	if item.ToolResult != nil {
		result := *item.ToolResult
		cloned.ToolResult = &result
	}
	return cloned
}

func cloneRawMessage(value []byte) []byte {
	return append([]byte(nil), value...)
}
