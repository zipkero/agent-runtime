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

// Status 는 Agent run의 현재 또는 종료 상태를 표현한다.
type Status string

const (
	StatusRunning     Status = "running"
	StatusFinal       Status = "final"
	StatusNeedsAction Status = "needs_action"
	StatusMaxSteps    Status = "max_steps"
	StatusError       Status = "error"
)

// TraceAction 은 Agent run 중 관찰 가능한 주요 상태 전이를 표현한다.
type TraceAction string

const (
	TraceActionUserMessage TraceAction = "user_message"
	TraceActionLLMRequest  TraceAction = "llm_request"
	TraceActionLLMResponse TraceAction = "llm_response"
	TraceActionFinal       TraceAction = "final"
	TraceActionNeedsAction TraceAction = "needs_action"
	TraceActionMaxSteps    TraceAction = "max_steps"
	TraceActionLLMError    TraceAction = "llm_error"
)

const defaultToolTimeout = 30 * time.Second

// Options 는 Agent run 실행에 필요한 provider-neutral 의존성과 정책이다.
type Options struct {
	Client      llm.LLMClient
	Model       string
	MaxSteps    int
	Tools       *tool.Registry
	ToolTimeout time.Duration
}

// Agent 는 메시지 상태를 소유하며 LLM 판단을 진행하는 Runtime 실행 객체다.
type Agent struct {
	client      llm.LLMClient
	model       string
	maxSteps    int
	tools       *tool.Registry
	toolTimeout time.Duration
}

// AgentState 는 호출자가 run 이후 메시지 누적과 종료 상태를 확인하는 값이다.
type AgentState struct {
	Messages    []message.Message
	Step        int
	Status      Status
	FinalAnswer string
	ToolCalls   []message.ToolCall
	LastError   error
	Trace       []TraceEvent
}

// TraceEvent 는 Agent run 중 메모리에 남기는 테스트 가능한 관찰 기록이다.
type TraceEvent struct {
	Step   int
	Action TraceAction
	Status Status
	Error  error
}

func New(opts Options) (*Agent, error) {
	if opts.Client == nil {
		return nil, errors.New("agent client is required")
	}
	toolTimeout := opts.ToolTimeout
	if toolTimeout == 0 {
		toolTimeout = defaultToolTimeout
	}

	return &Agent{
		client:      opts.Client,
		model:       opts.Model,
		maxSteps:    opts.MaxSteps,
		tools:       opts.Tools,
		toolTimeout: toolTimeout,
	}, nil
}

func (a *Agent) Run(ctx context.Context, input string) AgentState {
	state := AgentState{
		Status: StatusRunning,
	}
	state.Messages = append(state.Messages, message.User(input))
	state.record(TraceActionUserMessage, nil)

	for {
		if state.Step >= a.maxSteps {
			state.Status = StatusMaxSteps
			state.record(TraceActionMaxSteps, nil)
			return state
		}

		reqMessages := append([]message.Message(nil), state.Messages...)
		state.Step++
		state.record(TraceActionLLMRequest, nil)
		resp, err := a.client.Chat(ctx, llm.ChatRequest{
			Model:    a.model,
			Messages: reqMessages,
			Tools:    a.toolSchemas(),
		})
		if err != nil {
			state.Status = StatusError
			state.LastError = err
			state.record(TraceActionLLMError, err)
			return state
		}

		state.Messages = append(state.Messages, resp.Message)
		state.record(TraceActionLLMResponse, nil)
		if len(resp.Message.ToolCalls) == 0 {
			state.Status = StatusFinal
			state.FinalAnswer = resp.Message.Text
			state.record(TraceActionFinal, nil)
			return state
		}

		state.ToolCalls = append([]message.ToolCall(nil), resp.Message.ToolCalls...)
		if !a.hasTools() {
			state.Status = StatusNeedsAction
			state.record(TraceActionNeedsAction, nil)
			return state
		}

		for _, call := range resp.Message.ToolCalls {
			state.Messages = append(state.Messages, a.executeToolCall(ctx, call))
		}
	}
}

func (a *Agent) hasTools() bool {
	return len(a.toolSchemas()) > 0
}

func (a *Agent) toolSchemas() []message.ToolSchema {
	if a.tools == nil {
		return nil
	}
	return a.tools.Schemas()
}

func (a *Agent) executeToolCall(ctx context.Context, call message.ToolCall) message.Message {
	registeredTool, ok := a.tools.Lookup(call.Name)
	if !ok {
		return toolErrorMessage(call, fmt.Sprintf("tool %q is not registered", call.Name))
	}

	if err := registeredTool.Validate(call.Arguments); err != nil {
		return toolErrorMessage(call, err.Error())
	}

	toolCtx := ctx
	cancel := func() {}
	if a.toolTimeout > 0 {
		toolCtx, cancel = context.WithTimeout(ctx, a.toolTimeout)
	}
	defer cancel()

	result, err := registeredTool.Execute(toolCtx, call.Arguments)
	if err != nil {
		return toolErrorMessage(call, err.Error())
	}

	return message.Tool(message.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    result.Content,
	})
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
