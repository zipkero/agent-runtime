// Package agent는 LLM 판단과 Tool 실행을 반복하는 단일 Agent 실행 흐름을 제공한다.
package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// Status 타입은 Agent 실행의 현재 또는 종료 상태를 표현한다.
type Status string

const (
	StatusRunning     Status = "running"
	StatusFinal       Status = "final"
	StatusNeedsAction Status = "needs_action"
	StatusMaxSteps    Status = "max_steps"
	StatusError       Status = "error"
)

// TraceAction 타입은 Agent 실행 중 관찰 가능한 주요 상태 전이를 표현한다.
type TraceAction string

const (
	TraceActionUserMessage     TraceAction = "user_message"
	TraceActionLLMRequest      TraceAction = "llm_request"
	TraceActionLLMResponse     TraceAction = "llm_response"
	TraceActionFinal           TraceAction = "final"
	TraceActionNeedsAction     TraceAction = "needs_action"
	TraceActionMaxSteps        TraceAction = "max_steps"
	TraceActionLLMError        TraceAction = "llm_error"
	TraceActionMiddlewareError TraceAction = "middleware_error"
	TraceActionToolCall        TraceAction = "tool_call"
	TraceActionToolResult      TraceAction = "tool_result"
	TraceActionToolError       TraceAction = "tool_error"
	TraceActionToolTimeout     TraceAction = "tool_timeout"
	TraceActionExecutionLimit  TraceAction = "execution_limit"
)

const (
	defaultToolTimeout   = 30 * time.Second
	defaultMaxToolCalls  = 20
	limitMaxToolCalls    = "max_tool_calls"
	limitToolResultBytes = "max_tool_result_bytes"
	limitRunDeadline     = "run_deadline"
)

// Options 구조체는 Agent 실행에 필요한 공급자 중립 의존성과 제한 정책이다.
// ToolTimeout, MaxToolCalls, MaxToolResultBytes가 0이면 Runtime 기본값을 사용한다.
// MaxToolCalls 값과 MaxToolResultBytes 값은 음수를 허용하지 않는다.
// MaxSteps 값이 0 이하이면 LLM을 호출하지 않고 StatusMaxSteps 상태로 종료한다.
type Options struct {
	Client             llm.LLMClient
	Model              string
	MaxSteps           int
	Tools              *tool.Registry
	ToolTimeout        time.Duration
	MaxToolCalls       int
	MaxToolResultBytes int
}

type modelCallOptions struct {
	timeout    time.Duration
	middleware []ModelMiddleware
}

// Agent 구조체는 메시지 상태를 소유하며 LLM 판단을 진행하는 Runtime 실행 객체다.
type Agent struct {
	client             llm.LLMClient
	model              string
	maxSteps           int
	modelTimeout       time.Duration
	tools              *tool.Registry
	toolTimeout        time.Duration
	maxToolCalls       int
	maxToolResultBytes int
	middleware         []ModelMiddleware
}

// AgentState 구조체는 호출자가 실행 이후 메시지 누적과 종료 상태를 확인하는 값이다.
type AgentState struct {
	Messages    []message.Message
	Step        int
	Status      Status
	FinalAnswer string
	ToolCalls   []message.ToolCall
	LastError   error
	Trace       []TraceEvent
}

// TraceEvent 구조체는 Agent 실행 중 메모리에 남기는 관찰 기록이다.
type TraceEvent struct {
	Step       int
	Action     TraceAction
	Status     Status
	ToolCallID string
	ToolName   string
	IsError    bool
	Error      error
}

// New 함수는 주입된 의존성과 실행 제한으로 Agent를 만든다.
func New(opts Options) (*Agent, error) {
	return newAgent(opts, modelCallOptions{})
}

func newAgent(opts Options, modelCall modelCallOptions) (*Agent, error) {
	if opts.Client == nil {
		return nil, errors.New("agent client is required")
	}
	if err := validateModelMiddleware(modelCall.middleware); err != nil {
		return nil, err
	}
	if opts.MaxToolCalls < 0 {
		return nil, errors.New("agent max tool calls must not be negative")
	}
	if opts.MaxToolResultBytes < 0 {
		return nil, errors.New("agent max tool result bytes must not be negative")
	}
	toolTimeout := opts.ToolTimeout
	if toolTimeout == 0 {
		toolTimeout = defaultToolTimeout
	}
	maxToolCalls := opts.MaxToolCalls
	if maxToolCalls == 0 {
		maxToolCalls = defaultMaxToolCalls
	}
	maxToolResultBytes := opts.MaxToolResultBytes
	if maxToolResultBytes == 0 {
		maxToolResultBytes = tool.DefaultMaxResultBytes
	}

	return &Agent{
		client:             opts.Client,
		model:              opts.Model,
		maxSteps:           opts.MaxSteps,
		modelTimeout:       modelCall.timeout,
		tools:              opts.Tools,
		toolTimeout:        toolTimeout,
		maxToolCalls:       maxToolCalls,
		maxToolResultBytes: maxToolResultBytes,
		middleware:         append([]ModelMiddleware(nil), modelCall.middleware...),
	}, nil
}

// Run 메서드는 입력을 사용자 메시지로 추가하고 종료 조건에 도달할 때까지 LLM 판단과 Tool 실행을 반복한다.
// 실행 오류와 제한 초과는 반환 오류 대신 AgentState의 Status와 LastError에 보존한다.
func (a *Agent) Run(ctx context.Context, input string) AgentState {
	state := AgentState{
		Status: StatusRunning,
	}
	state.Messages = append(state.Messages, message.User(input))
	state.record(TraceActionUserMessage, nil)
	toolCallCount := 0

	for {
		if err := executionLimitFromContext(ctx); err != nil {
			state.stopExecutionLimit(err)
			return state
		}
		if state.Step >= a.maxSteps {
			state.Status = StatusMaxSteps
			state.record(TraceActionMaxSteps, nil)
			return state
		}

		state.Step++
		modelRequest, err := applyPreModelMiddleware(ctx, a.middleware, llm.ChatRequest{
			Model:    a.model,
			Messages: cloneMessages(state.Messages),
			Tools:    a.toolSchemas(),
		})
		if err != nil {
			if limitErr := executionLimitFromContext(ctx); limitErr != nil {
				state.stopExecutionLimit(limitErr)
				return state
			}
			state.Status = StatusError
			state.LastError = err
			state.record(TraceActionMiddlewareError, err)
			return state
		}
		state.record(TraceActionLLMRequest, nil)

		callCtx := ctx
		cancel := func() {}
		if a.modelTimeout > 0 {
			callCtx, cancel = context.WithTimeout(ctx, a.modelTimeout)
		}
		providerResponse, err := a.client.Chat(callCtx, modelRequest)
		cancel()
		if err != nil {
			if limitErr := executionLimitFromContext(ctx); limitErr != nil {
				state.stopExecutionLimit(limitErr)
				return state
			}
			state.Status = StatusError
			state.LastError = err
			state.record(TraceActionLLMError, err)
			return state
		}

		finalResponse, err := applyPostModelMiddleware(ctx, a.middleware, modelRequest, providerResponse)
		if err != nil {
			if limitErr := executionLimitFromContext(ctx); limitErr != nil {
				state.stopExecutionLimit(limitErr)
				return state
			}
			state.Status = StatusError
			state.LastError = err
			state.record(TraceActionMiddlewareError, err)
			return state
		}
		if err := executionLimitFromContext(ctx); err != nil {
			state.stopExecutionLimit(err)
			return state
		}

		state.Messages = append(state.Messages, finalResponse.Message)
		state.ToolCalls = append([]message.ToolCall(nil), finalResponse.Message.ToolCalls...)
		state.record(TraceActionLLMResponse, nil)
		if len(state.ToolCalls) == 0 {
			state.Status = StatusFinal
			state.FinalAnswer = finalResponse.Message.Text
			state.record(TraceActionFinal, nil)
			return state
		}

		if !a.hasTools() {
			state.Status = StatusNeedsAction
			state.record(TraceActionNeedsAction, nil)
			return state
		}

		for _, call := range finalResponse.Message.ToolCalls {
			if err := executionLimitFromContext(ctx); err != nil {
				state.stopExecutionLimit(err)
				return state
			}
			toolCallCount++
			if toolCallCount > a.maxToolCalls {
				state.stopExecutionLimit(executionLimitError(limitMaxToolCalls, toolCallCount, a.maxToolCalls, nil))
				return state
			}

			toolMessage, err := a.executeToolCall(ctx, &state, call)
			if err != nil {
				state.stopExecutionLimit(err)
				return state
			}
			state.Messages = append(state.Messages, toolMessage)
		}
	}
}

func (a *Agent) hasTools() bool {
	return a.tools != nil && a.tools.Len() > 0
}

func (a *Agent) toolSchemas() []message.ToolSchema {
	if a.tools == nil {
		return nil
	}
	return a.tools.Schemas()
}

// cloneMessages 함수는 middleware가 내부 대화 상태를 별칭으로 보관하거나 수정하지 못하도록 요청 소유권을 분리한다.
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
		cloned.ToolCalls[i].Arguments = append([]byte(nil), call.Arguments...)
	}
	if item.ToolResult != nil {
		result := *item.ToolResult
		cloned.ToolResult = &result
	}
	return cloned
}

// executeToolCall 메서드는 Tool 자체의 실패를 오류 메시지로 바꿔 다음 LLM 판단을 계속하고, 실행 전체 제한만 상위로 반환한다.
func (a *Agent) executeToolCall(ctx context.Context, state *AgentState, call message.ToolCall) (message.Message, error) {
	state.recordTool(TraceActionToolCall, call, false, nil)

	registeredTool, ok := a.tools.Lookup(call.Name)
	if !ok {
		err := fmt.Errorf("tool %q is not registered", call.Name)
		return a.toolErrorResult(state, call, TraceActionToolError, err), nil
	}

	if err := registeredTool.Validate(call.Arguments); err != nil {
		return a.toolErrorResult(state, call, TraceActionToolError, err), nil
	}

	toolCtx := ctx
	cancel := func() {}
	if a.toolTimeout > 0 {
		toolCtx, cancel = context.WithTimeout(ctx, a.toolTimeout)
	}
	defer cancel()

	result, err := registeredTool.Execute(toolCtx, call.Arguments)
	if limitErr := executionLimitFromContext(ctx); limitErr != nil {
		return message.Message{}, limitErr
	}
	if err != nil {
		action := TraceActionToolError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(toolCtx.Err(), context.DeadlineExceeded) {
			action = TraceActionToolTimeout
		}
		return a.toolErrorResult(state, call, action, err), nil
	}
	if err := toolCtx.Err(); err != nil {
		action := TraceActionToolError
		if errors.Is(err, context.DeadlineExceeded) {
			action = TraceActionToolTimeout
		}
		return a.toolErrorResult(state, call, action, err), nil
	}
	if size := len(result.Content); size > a.maxToolResultBytes {
		err := executionLimitError(limitToolResultBytes, size, a.maxToolResultBytes, nil)
		state.recordTool(TraceActionToolError, call, true, err)
		return toolErrorMessage(call, err.Error()), nil
	}

	state.recordTool(TraceActionToolResult, call, false, nil)
	return message.Tool(message.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    result.Content,
	}), nil
}

func (a *Agent) toolErrorResult(state *AgentState, call message.ToolCall, action TraceAction, err error) message.Message {
	if size := len(err.Error()); size > a.maxToolResultBytes {
		err = executionLimitError(limitToolResultBytes, size, a.maxToolResultBytes, nil)
		action = TraceActionToolError
	}
	state.recordTool(action, call, true, err)
	return toolErrorMessage(call, err.Error())
}

func executionLimitFromContext(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return executionLimitError(limitRunDeadline, 0, 0, context.DeadlineExceeded)
	}
	return nil
}

func toolErrorMessage(call message.ToolCall, content string) message.Message {
	return message.Tool(message.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    content,
		IsError:    true,
	})
}

func (s *AgentState) record(action TraceAction, err error) {
	s.Trace = append(s.Trace, TraceEvent{
		Step:   s.Step,
		Action: action,
		Status: s.Status,
		Error:  err,
	})
}

func (s *AgentState) stopExecutionLimit(err error) {
	s.Status = StatusError
	s.LastError = err
	s.record(TraceActionExecutionLimit, err)
}

func (s *AgentState) recordTool(action TraceAction, call message.ToolCall, isError bool, err error) {
	s.Trace = append(s.Trace, TraceEvent{
		Step:       s.Step,
		Action:     action,
		Status:     s.Status,
		ToolCallID: call.ID,
		ToolName:   call.Name,
		IsError:    isError,
		Error:      err,
	})
}
