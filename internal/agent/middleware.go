package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zipkero/agent-runtime/internal/llm"
)

// MiddlewareStage 타입은 모델 호출에서 middleware가 실행되는 경계를 식별한다.
type MiddlewareStage string

const (
	// MiddlewareStagePreModel 상수는 공급자 호출 전에 요청을 처리하는 경계다.
	MiddlewareStagePreModel MiddlewareStage = "pre_model"
	// MiddlewareStagePostModel 상수는 공급자 응답을 정규화한 뒤 처리하는 경계다.
	MiddlewareStagePostModel MiddlewareStage = "post_model"
)

// RunnerErrorKind 타입은 Runner 실행 경계에서 분류하는 오류 종류다.
type RunnerErrorKind string

const (
	// RunnerErrorKindMiddleware 상수는 middleware hook이 반환한 실패를 나타내며 Stage와 Middleware 이름을 함께 보존한다.
	RunnerErrorKindMiddleware RunnerErrorKind = "middleware"
	// RunnerErrorKindStructuredOutput 상수는 schema compile, JSON 해석, schema 검증 실패를 나타낸다.
	RunnerErrorKindStructuredOutput RunnerErrorKind = "structured_output"
	// RunnerErrorKindExecutionLimit 상수는 Tool 호출 수, result 크기, 실행 deadline 제한 초과를 나타낸다.
	RunnerErrorKindExecutionLimit RunnerErrorKind = "execution_limit"
	// RunnerErrorKindIncompleteResponse 상수는 완료 사유가 정상 완료도 Tool 호출도 아닌 응답을 나타낸다.
	RunnerErrorKindIncompleteResponse RunnerErrorKind = "incomplete_response"
	// RunnerErrorKindUnsupportedStream 상수는 streaming을 지원하지 않는 LLMClient로 RunStream을 호출했음을 나타낸다.
	RunnerErrorKindUnsupportedStream RunnerErrorKind = "unsupported_stream"
)

// StructuredOutputOperation 타입은 structured output 처리 중 실패한 단계를 식별한다.
type StructuredOutputOperation string

const (
	// StructuredOutputOperationSchemaCompile 상수는 Runner 생성 시 OutputSchema compile 단계의 실패를 식별한다.
	StructuredOutputOperationSchemaCompile StructuredOutputOperation = "schema_compile"
	// StructuredOutputOperationJSONParse 상수는 최종 답을 단일 JSON 문서로 해석하는 단계의 실패를 식별한다.
	StructuredOutputOperationJSONParse StructuredOutputOperation = "json_parse"
	// StructuredOutputOperationValidation 상수는 해석한 JSON을 schema로 검증하는 단계의 실패를 식별한다.
	StructuredOutputOperationValidation StructuredOutputOperation = "validation"
)

// RunnerError 구조체는 Runner 고유 실패 분류와 원인을 함께 보존한다.
type RunnerError struct {
	Kind         RunnerErrorKind
	Operation    StructuredOutputOperation
	Stage        MiddlewareStage
	Middleware   string
	Limit        string
	Current      int
	Maximum      int
	FinishReason llm.FinishReason
	StopReason   string
	Err          error
}

func (e *RunnerError) Error() string {
	if e.Kind == RunnerErrorKindStructuredOutput {
		return fmt.Sprintf("agent runner %s %s: %v", e.Kind, e.Operation, e.Err)
	}
	if e.Kind == RunnerErrorKindExecutionLimit {
		if e.Maximum > 0 {
			return fmt.Sprintf("agent runner %s %s: current %d exceeds maximum %d", e.Kind, e.Limit, e.Current, e.Maximum)
		}
		return fmt.Sprintf("agent runner %s %s: %v", e.Kind, e.Limit, e.Err)
	}
	if e.Kind == RunnerErrorKindIncompleteResponse {
		return fmt.Sprintf(
			"agent runner %s: finish reason %q from provider stop reason %q",
			e.Kind,
			e.FinishReason,
			e.StopReason,
		)
	}
	if e.Kind == RunnerErrorKindUnsupportedStream {
		return fmt.Sprintf("agent runner %s: client does not implement llm.StreamingLLMClient", e.Kind)
	}
	return fmt.Sprintf("agent runner %s %s middleware %q: %v", e.Kind, e.Stage, e.Middleware, e.Err)
}

func (e *RunnerError) Unwrap() error {
	return e.Err
}

// IsRunnerErrorKind 함수는 오류 체인에 지정한 Runner 오류 종류가 있는지 확인한다.
func IsRunnerErrorKind(err error, kind RunnerErrorKind) bool {
	var runnerErr *RunnerError
	return errors.As(err, &runnerErr) && runnerErr.Kind == kind
}

// PreModelHook 함수 타입은 Agent 상태에서 분리된 현재 요청을 처리한다. 반환한 요청은 소유권이 이전되므로 이후 수정하지 않는다.
type PreModelHook func(context.Context, llm.ChatRequest) (llm.ChatRequest, error)

// PostModelHook 함수 타입은 실제 모델 요청을 읽기 전용으로 관찰하고 현재 응답을 처리한다.
// 반환한 응답은 소유권이 이전되므로 이후 수정하지 않는다.
type PostModelHook func(context.Context, llm.ChatRequest, llm.ChatResponse) (llm.ChatResponse, error)

// ModelMiddleware 구조체는 모델 요청과 정규화된 응답을 등록 순서대로 처리하는 선택적 hook 묶음이다.
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

func executionLimitError(limit string, current, maximum int, err error) error {
	return &RunnerError{
		Kind:    RunnerErrorKindExecutionLimit,
		Limit:   limit,
		Current: current,
		Maximum: maximum,
		Err:     err,
	}
}

func incompleteResponseError(finishReason llm.FinishReason, stopReason string) error {
	return &RunnerError{
		Kind:         RunnerErrorKindIncompleteResponse,
		FinishReason: finishReason,
		StopReason:   stopReason,
	}
}

func structuredOutputError(operation StructuredOutputOperation, err error) error {
	return &RunnerError{
		Kind:      RunnerErrorKindStructuredOutput,
		Operation: operation,
		Err:       err,
	}
}

func unsupportedStreamError() error {
	return &RunnerError{Kind: RunnerErrorKindUnsupportedStream}
}
