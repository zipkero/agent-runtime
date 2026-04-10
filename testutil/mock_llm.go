package testutil

import (
	"context"
	"fmt"

	"github.com/zipkero/gogent/internal/llm"
)

// MockLLMClient 는 테스트에서 실제 LLM API 호출 없이 응답을 제어하기 위한 mock 구현체다.
// 시나리오 기반으로 Complete() 호출 순서에 따라 미리 등록한 응답을 순서대로 반환한다.
// 프로덕션 코드에서 import 금지.
type MockLLMClient struct {
	// responses 는 순서대로 반환할 응답 목록이다.
	responses []mockResponse
	// callCount 는 Complete() 가 호출된 총 횟수다.
	callCount int
}

// mockResponse 는 한 번의 Complete() 호출에 대한 응답 또는 에러를 담는다.
type mockResponse struct {
	resp llm.CompletionResponse
	err  error
}

// NewMockLLMClient 는 MockLLMClient 를 생성한다.
// WithResponse / WithError 로 응답을 등록한 뒤 사용한다.
func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{}
}

// WithResponse 는 다음 Complete() 호출에 대해 성공 응답을 등록한다.
// 메서드 체이닝을 지원한다.
func (m *MockLLMClient) WithResponse(content string) *MockLLMClient {
	m.responses = append(m.responses, mockResponse{
		resp: llm.CompletionResponse{
			Content:      content,
			FinishReason: "stop",
			Usage: llm.TokenUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		},
	})
	return m
}

// WithError 는 다음 Complete() 호출에 대해 에러를 등록한다.
// 메서드 체이닝을 지원한다.
func (m *MockLLMClient) WithError(err error) *MockLLMClient {
	m.responses = append(m.responses, mockResponse{err: err})
	return m
}

// Complete 는 등록된 응답을 순서대로 반환한다.
// 등록된 응답이 소진되면 에러를 반환한다.
func (m *MockLLMClient) Complete(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	idx := m.callCount
	m.callCount++

	if idx >= len(m.responses) {
		return llm.CompletionResponse{}, fmt.Errorf("MockLLMClient: 등록된 응답 소진 (호출 횟수: %d, 등록 수: %d)", idx+1, len(m.responses))
	}

	r := m.responses[idx]
	return r.resp, r.err
}

// CallCount 는 지금까지 Complete() 가 호출된 횟수를 반환한다.
func (m *MockLLMClient) CallCount() int {
	return m.callCount
}
