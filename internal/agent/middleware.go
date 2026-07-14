package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zipkero/agent-runtime/internal/llm"
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

// PreModelHook 은 Agent 상태에서 분리된 현재 요청을 처리한다. 반환한 요청은 소유권이 이전되므로 이후 수정하지 않는다.
type PreModelHook func(context.Context, llm.ChatRequest) (llm.ChatRequest, error)

// PostModelHook 은 실제 model 요청을 읽기 전용으로 관찰하고 현재 응답을 처리한다.
// 반환한 응답은 소유권이 이전되므로 이후 수정하지 않는다.
type PostModelHook func(context.Context, llm.ChatRequest, llm.ChatResponse) (llm.ChatResponse, error)

// ModelMiddleware 는 model 요청과 정규화된 응답을 순차 처리하는 선택적 hook 묶음이다.
type ModelMiddleware struct {
	Name      string
	PreModel  PreModelHook
	PostModel PostModelHook
}

func validateModelMiddleware(middleware []ModelMiddleware) error {
	names := make(map[string]struct{}, len(middleware))
	for i, item := range middleware {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return fmt.Errorf("agent runner middleware[%d] name is required", i)
		}
		if name != item.Name {
			return fmt.Errorf("agent runner middleware[%d] name must not have leading or trailing whitespace", i)
		}
		if item.PreModel == nil && item.PostModel == nil {
			return fmt.Errorf("agent runner middleware %q requires a pre-model or post-model hook", item.Name)
		}
		if _, exists := names[name]; exists {
			return fmt.Errorf("agent runner middleware name %q is duplicated", name)
		}
		names[name] = struct{}{}
	}
	return nil
}

func applyPreModelMiddleware(ctx context.Context, middlewares []ModelMiddleware, request llm.ChatRequest) (llm.ChatRequest, error) {
	currentRequest := request
	for _, middleware := range middlewares {
		if middleware.PreModel == nil {
			continue
		}

		updatedRequest, err := middleware.PreModel(ctx, currentRequest)
		if err != nil {
			return llm.ChatRequest{}, middlewareError(MiddlewareStagePreModel, middleware.Name, err)
		}
		currentRequest = updatedRequest
	}
	return currentRequest, nil
}

func applyPostModelMiddleware(
	ctx context.Context,
	middlewares []ModelMiddleware,
	request llm.ChatRequest,
	response llm.ChatResponse,
) (llm.ChatResponse, error) {
	currentResponse := response
	for _, middleware := range middlewares {
		if middleware.PostModel == nil {
			continue
		}

		updatedResponse, err := middleware.PostModel(ctx, request, currentResponse)
		if err != nil {
			return llm.ChatResponse{}, middlewareError(MiddlewareStagePostModel, middleware.Name, err)
		}
		currentResponse = updatedResponse
	}

	return currentResponse, nil
}

func middlewareError(stage MiddlewareStage, name string, err error) error {
	return &RunnerError{
		Kind:       RunnerErrorKindMiddleware,
		Stage:      stage,
		Middleware: name,
		Err:        err,
	}
}
