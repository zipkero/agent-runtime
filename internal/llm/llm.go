// Package llm 은 provider-neutral LLM client 계약을 정의한다.
package llm

import (
	"context"
	"fmt"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/message"
)

// LLMClient 는 호출자가 chat 응답을 요청할 때 의존하는 단일 계약이다.
type LLMClient interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// LLMStreamer 는 provider가 streaming chat 응답을 지원할 때 선택적으로 구현하는 계약이다.
type LLMStreamer interface {
	Stream(ctx context.Context, req ChatRequest) (ChatStream, error)
}

// ChatStream 은 provider-neutral streaming 응답을 순차적으로 읽는 reader다.
type ChatStream interface {
	Recv() (ChatStreamEvent, error)
	Close() error
}

// ChatStreamEventType 은 streaming 응답에서 관찰 가능한 event 종류다.
type ChatStreamEventType string

const (
	// ChatStreamEventTypeTextDelta 는 최종 assistant message로 조립되기 전의 text chunk다.
	ChatStreamEventTypeTextDelta ChatStreamEventType = "text_delta"
	// ChatStreamEventTypeMessageComplete 는 provider stream이 완료된 뒤 조립된 assistant message다.
	ChatStreamEventTypeMessageComplete ChatStreamEventType = "message_complete"
)

// ChatStreamEvent 는 provider-specific streaming 응답을 runtime 내부 공통 event로 정규화한 값이다.
type ChatStreamEvent struct {
	Type      ChatStreamEventType
	TextDelta string
	Message   message.Message
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

// NewClient 는 설정된 provider에 맞는 LLMClient 구현체를 생성한다.
func NewClient(cfg config.Config) (LLMClient, error) {
	switch cfg.Provider {
	case config.ProviderOllama:
		return NewOllamaClient(cfg)
	case config.ProviderClaude:
		return NewClaudeClient(cfg)
	default:
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
}
