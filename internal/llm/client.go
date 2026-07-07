package llm

import (
	"context"

	"github.com/zipkero/agent-runtime/internal/message"
)

// LLMClient 는 provider 구현을 Runtime 호출 contract 뒤에 숨긴다.
type LLMClient interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// ChatRequest 는 provider-neutral 단발 LLM 요청이다.
type ChatRequest struct {
	Model    string
	Messages []message.Message
	Tools    []message.ToolSchema
}

// ChatResponse 는 provider 응답을 Runtime 내부 메시지 형태로 정규화한 결과다.
type ChatResponse struct {
	Provider   Provider
	Model      string
	Message    message.Message
	StopReason string
	Usage      Usage
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}
