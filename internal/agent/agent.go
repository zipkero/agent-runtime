// Package agent 는 ReAct loop와 그 실행 상태를 정의한다.
// llm.LLMClient(interface)와 message 타입에만 의존하며, provider 구현체에는 의존하지 않는다.
package agent

import (
	"context"
	"time"

	"github.com/zipkero/agent-runtime/internal/llm"
	"github.com/zipkero/agent-runtime/internal/message"
	"github.com/zipkero/agent-runtime/internal/tool"
)

// Status 는 AgentState가 놓인 종료 종류를 하나의 명시적 값으로 구분한다.
// running을 제외한 final/max steps/error 세 값이 종료 상태이며,
// loop는 종료 상태에 도달하면 더 이상 LLM을 호출하지 않는다.
type Status string

const (
	// StatusRunning 은 아직 최종 답에 도달하지 않았고 step 여유가 남아 loop가 도는 중인 상태다.
	StatusRunning Status = "running"
	// StatusFinal 은 tool_call이 없는 assistant 응답을 받아 최종 답으로 판정해 종료한 상태다.
	StatusFinal Status = "final"
	// StatusMaxSteps 는 step counter가 상한에 도달했는데도 최종 답에 못 닿아 안전 종료한 상태다.
	StatusMaxSteps Status = "max_steps"
	// StatusError 는 LLM 호출이 에러를 반환해 안전 종료한 상태다. 원인은 Err에 담는다.
	StatusError Status = "error"
)

// AgentState 는 한 번의 Agent 실행 동안 누적되는 값이자 종료 후 호출자가 결과를 관찰하는 표면이다.
type AgentState struct {
	// Messages 는 user 입력부터 매 step의 assistant 응답까지 누적된 대화다.
	Messages []message.Message
	Steps    int
	Status   Status
	// Err 는 Status==StatusError일 때만 채워지는 원인 에러다.
	Err error
}

// FinalMessage 는 final 상태일 때 최종 답 메시지(마지막 assistant 응답)와 true를 반환한다.
// final이 아니거나 assistant 응답이 없으면 zero 값과 false를 반환한다.
func (s AgentState) FinalMessage() (message.Message, bool) {
	if s.Status != StatusFinal {
		return message.Message{}, false
	}
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == message.RoleAssistant {
			return s.Messages[i], true
		}
	}
	return message.Message{}, false
}

// ReflectionHook 은 매 step 경계에서 현재 step 번호와 누적 state를 받아 관찰하는 콜백 타입이다.
// nil을 주입하면 no-op으로 동작하며, loop가 항상 정상 진행된다.
type ReflectionHook func(step int, state AgentState)

// Agent 는 주입된 LLMClient를 들고 AgentState 위에서 ReAct loop를 실행하는 단위다.
type Agent struct {
	client      llm.LLMClient
	model       string
	maxSteps    int
	hook        ReflectionHook
	registry    *tool.Registry  // tool 이름 조회와 schema 수집에 사용한다. nil이면 tool 미사용.
	toolTimeout time.Duration   // per-tool 실행 deadline. 0이면 context 상속만 따른다.
}

// NewAgent 는 Agent를 생성한다.
// hook이 nil이면 step 경계에서 아무 동작도 하지 않는다(no-op).
// registry가 nil이면 tool schema를 LLM에 싣지 않고 tool_call 결과도 처리하지 않는다.
// toolTimeout이 0이면 per-tool deadline을 별도로 적용하지 않는다(loop ctx 상속만).
func NewAgent(client llm.LLMClient, model string, maxSteps int, hook ReflectionHook, registry *tool.Registry, toolTimeout time.Duration) *Agent {
	return &Agent{
		client:      client,
		model:       model,
		maxSteps:    maxSteps,
		hook:        hook,
		registry:    registry,
		toolTimeout: toolTimeout,
	}
}

// Run 은 사용자 입력(prompt)을 첫 메시지로 ReAct loop를 실행하고,
// final/max steps/error 중 하나로 종료된 AgentState를 반환한다.
// 에러는 두 번째 반환값으로 던지지 않고 state에 흡수된다(ctx 취소 포함).
func (a *Agent) Run(ctx context.Context, prompt string) AgentState {
	// (1) 초기화: user 입력을 state에 넣고 step 0, 상태 running으로 시작
	state := AgentState{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock(prompt)},
			},
		},
		Steps:  0,
		Status: StatusRunning,
	}

	for {
		// (2) step 경계 — reflection hook 호출(nil이면 no-op)
		if a.hook != nil {
			a.hook(state.Steps, state)
		}

		// (3) max step 선검사 — LLM 호출 전에 상한 도달 여부를 판정
		if state.Steps >= a.maxSteps {
			state.Status = StatusMaxSteps
			return state
		}

		// (4) LLM 호출 — ctx를 그대로 전파하고, 에러는 state에 흡수
		resp, err := a.client.Chat(ctx, llm.ChatRequest{
			Model:    a.model,
			Messages: state.Messages,
		})
		if err != nil {
			state.Status = StatusError
			state.Err = err
			return state
		}

		// (5) assistant 응답 누적, step counter 증가
		state.Messages = append(state.Messages, resp.Message)
		state.Steps++

		// (6) 종료 판정 — tool_call이 없으면 final, 있으면 running 유지(미실행, 신호로만)
		if !resp.Message.HasToolCalls() {
			state.Status = StatusFinal
			return state
		}
		// tool_call이 있으면 running 유지하며 다음 회전으로
	}
}
