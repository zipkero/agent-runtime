// Package llm 은 provider-neutral LLM client 계약을 정의한다.
package llm

import (
	"context"

	"github.com/zipkero/agent-runtime/internal/message"
)

// LLMClient 는 호출자가 chat 응답을 요청할 때 의존하는 단일 계약이다.
type LLMClient interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// ChatRequest 는 단일 chat 호출에 필요한 provider-neutral 입력이다.
type ChatRequest struct {
	Model    string
	Messages []message.Message
	Tools    []message.ToolSpec
}

// ChatResponse 는 LLM provider가 반환한 assistant 메시지를 담는다.
type ChatResponse struct {
	Message message.Message
}
