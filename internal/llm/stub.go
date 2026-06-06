package llm

import "context"

var _ LLMClient = (*StubClient)(nil)

// StubClient 는 테스트에서 실제 provider 호출 없이 미리 정한 결과를 반환하는 LLMClient다.
type StubClient struct {
	Response ChatResponse
	Err      error
}

// NewStubClient 는 항상 response를 반환하는 StubClient를 생성한다.
func NewStubClient(response ChatResponse) *StubClient {
	return &StubClient{Response: response}
}

// NewErrorStubClient 는 항상 err를 반환하는 StubClient를 생성한다.
func NewErrorStubClient(err error) *StubClient {
	return &StubClient{Err: err}
}

func (c *StubClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// 실제 client처럼 취소된 ctx를 먼저 존중해, 호출부의 취소·timeout 동작을 stub으로도 재현한다.
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}
	if c.Err != nil {
		return ChatResponse{}, c.Err
	}

	return c.Response, nil
}
