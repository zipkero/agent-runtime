// Package agent 는 ReAct loop와 그 실행 상태를 정의한다.
// llm.LLMClient(interface)와 message 타입에만 의존하며, provider 구현체에는 의존하지 않는다.
package agent

import "github.com/zipkero/agent-runtime/internal/message"

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
