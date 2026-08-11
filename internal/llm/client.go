// Package llm은 공급자별 LLM API를 Runtime이 사용하는 공통 계약 뒤에 둔다.
package llm

import (
	"context"
	"iter"

	"github.com/zipkero/agent-runtime/internal/message"
)

// LLMClient 인터페이스는 공급자 구현을 Runtime 호출 계약 뒤에 숨긴다.
// Chat 구현은 req를 읽기 전용으로 사용하고 참조를 보관하지 않으며, 반환한 응답의 소유권은 호출자에게 이전한다.
type LLMClient interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// ChatStreamEventKind 타입은 StreamChat이 내보내는 stream event 종류를 구분한다.
type ChatStreamEventKind string

const (
	// ChatStreamEventTextDelta 상수는 생성 중인 assistant text 조각을 나타낸다.
	ChatStreamEventTextDelta ChatStreamEventKind = "text_delta"
	// ChatStreamEventResponse 상수는 stream이 끝난 뒤 조립한 완성 응답을 나타낸다.
	ChatStreamEventResponse ChatStreamEventKind = "response"
)

// ChatStreamEvent 구조체는 공급자 중립 stream 조각 하나를 나타낸다.
// Kind가 ChatStreamEventTextDelta이면 TextDelta만 채워지고, ChatStreamEventResponse이면 Response만 채워진다.
type ChatStreamEvent struct {
	Kind      ChatStreamEventKind
	TextDelta string
	Response  *ChatResponse
}

// StreamingLLMClient 인터페이스는 LLMClient에 streaming 호출을 추가한다.
// StreamChat이 반환하는 iterator는 0개 이상의 text delta 뒤에 완성된 response를 정확히 한 번 내보내고 끝난다.
// 오류가 발생하면 오류 하나를 내보내고 response는 내보내지 않는다. Consumer가 순회를 중단하면 추가 event 없이
// 내부 연결과 요청이 정리된다. req와 반환한 event·response의 소유권 규칙은 LLMClient.Chat과 동일하다.
type StreamingLLMClient interface {
	LLMClient
	StreamChat(ctx context.Context, req ChatRequest) iter.Seq2[ChatStreamEvent, error]
}

// ChatRequest 구조체는 공급자 중립 단발 LLM 요청이다.
type ChatRequest struct {
	Model    string
	Messages []message.Message
	Tools    []message.ToolSchema
}

// FinishReason 타입은 공급자별 완료 사유를 Runtime이 상태 전이에 사용하는 공통 값으로 정규화한다.
type FinishReason string

const (
	// FinishReasonComplete 상수는 공급자가 응답을 끝까지 생성해 정상 완료했음을 나타낸다.
	FinishReasonComplete FinishReason = "complete"
	// FinishReasonToolCall 상수는 공급자가 Tool 실행 결과를 기다리며 응답을 끝냈음을 나타낸다.
	FinishReasonToolCall FinishReason = "tool_call"
	// FinishReasonLengthLimit 상수는 출력 토큰 상한에 걸려 응답이 잘렸음을 나타낸다.
	FinishReasonLengthLimit FinishReason = "length_limit"
	// FinishReasonBlocked 상수는 공급자가 안전 정책 등으로 응답 생성을 거부했음을 나타낸다.
	FinishReasonBlocked FinishReason = "blocked"
	// FinishReasonUnknown 상수는 공급자 원문을 위 값으로 정규화할 수 없었음을 나타내며 StopReason에 원문이 남는다.
	FinishReasonUnknown FinishReason = "unknown"
)

// ChatResponse 구조체는 공급자 응답을 Runtime 내부 메시지 형태로 정규화한 결과다.
// FinishReason은 Runtime 상태 전이용 공통 값이고 StopReason은 진단을 위한 공급자 원문이다.
// 기존 custom LLMClient는 FinishReason을 비워 둘 수 있으며 Agent는 이를 정상 완료로 해석한다.
type ChatResponse struct {
	Provider     Provider
	Model        string
	Message      message.Message
	FinishReason FinishReason
	StopReason   string
	Usage        Usage
}

// Usage 구조체는 한 번의 LLM 호출에서 소비한 토큰 수를 보존한다.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}
