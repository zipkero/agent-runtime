package agent

import (
	"context"
	"errors"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
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

// Options 는 Agent run 실행에 필요한 provider-neutral 의존성과 정책이다.
type Options struct {
	Client   llm.LLMClient
	Model    string
	MaxSteps int
}

// Agent 는 메시지 상태를 소유하며 LLM 판단을 진행하는 Runtime 실행 객체다.
type Agent struct {
	client   llm.LLMClient
	model    string
	maxSteps int
}

// AgentState 는 호출자가 run 이후 메시지 누적과 종료 상태를 확인하는 값이다.
type AgentState struct {
	Messages    []message.Message
	Step        int
	Status      Status
	FinalAnswer string
	ToolCalls   []message.ToolCall
	LastError   error
}

func New(opts Options) (*Agent, error) {
	if opts.Client == nil {
		return nil, errors.New("agent client is required")
	}

	return &Agent{
		client:   opts.Client,
		model:    opts.Model,
		maxSteps: opts.MaxSteps,
	}, nil
}

func (a *Agent) Run(ctx context.Context, input string) AgentState {
	state := AgentState{
		Status: StatusRunning,
	}
	state.Messages = append(state.Messages, message.User(input))

	if state.Step >= a.maxSteps {
		state.Status = StatusMaxSteps
		return state
	}

	reqMessages := append([]message.Message(nil), state.Messages...)
	state.Step++
	resp, err := a.client.Chat(ctx, llm.ChatRequest{
		Model:    a.model,
		Messages: reqMessages,
	})
	if err != nil {
		state.Status = StatusError
		state.LastError = err
		return state
	}

	state.Messages = append(state.Messages, resp.Message)
	if len(resp.Message.ToolCalls) > 0 {
		state.Status = StatusNeedsAction
		state.ToolCalls = append([]message.ToolCall(nil), resp.Message.ToolCalls...)
		return state
	}

	state.Status = StatusFinal
	state.FinalAnswer = resp.Message.Text
	return state
}
