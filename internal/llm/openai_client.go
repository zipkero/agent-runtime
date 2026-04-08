package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	openAIEndpoint = "https://api.openai.com/v1/chat/completions"
	defaultTimeout = 60 * time.Second
	defaultModel   = "gpt-5.4-nano"
)

// OpenAIClient 는 OpenAI Chat Completions API를 호출하는 LLMClient 구현체다.
type OpenAIClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
	timeout    time.Duration
}

// OpenAIOption 은 OpenAIClient 생성 시 선택 옵션을 지정하는 함수 타입이다.
type OpenAIOption func(*OpenAIClient)

// WithTimeout 은 LLM API 호출 당 timeout을 설정한다.
// 기본값: 60s
func WithTimeout(d time.Duration) OpenAIOption {
	return func(c *OpenAIClient) {
		c.timeout = d
	}
}

// WithModel 은 사용할 모델을 설정한다.
// 기본값: defaultModel
func WithModel(model string) OpenAIOption {
	return func(c *OpenAIClient) {
		if model != "" {
			c.model = model
		}
	}
}

// WithHTTPClient 는 커스텀 http.Client를 주입한다. (테스트 또는 프록시 용도)
func WithHTTPClient(hc *http.Client) OpenAIOption {
	return func(c *OpenAIClient) {
		c.httpClient = hc
	}
}

// NewOpenAIClient 는 OpenAIClient를 생성한다.
// apiKey: OpenAI API 키 (필수)
func NewOpenAIClient(apiKey string, opts ...OpenAIOption) *OpenAIClient {
	c := &OpenAIClient{
		apiKey:     apiKey,
		model:      defaultModel,
		httpClient: &http.Client{},
		timeout:    defaultTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// openAIRequest 는 OpenAI Chat Completions API의 요청 body 구조체다.
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse 는 OpenAI Chat Completions API의 응답 body 구조체다.
type openAIResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *openAIError `json:"error,omitempty"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// Complete 는 LLMClient 인터페이스를 구현한다.
// 개별 LLM 호출 단위로 context.WithTimeout을 적용해 goroutine 무기한 대기를 방지한다.
func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	messages := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openAIMessage{Role: m.Role, Content: m.Content}
	}

	body := openAIRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("openai: 요청 직렬화 실패: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, openAIEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("openai: HTTP 요청 생성 실패: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	calledAt := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("openai: HTTP 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("openai: 응답 body 읽기 실패: %w", err)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return CompletionResponse{}, fmt.Errorf("openai: 응답 파싱 실패: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if apiResp.Error != nil {
			msg = fmt.Sprintf("HTTP %d: %s (type=%s, code=%s)", resp.StatusCode, apiResp.Error.Message, apiResp.Error.Type, apiResp.Error.Code)
		}
		return CompletionResponse{}, fmt.Errorf("openai: API 오류 — %s", msg)
	}

	if len(apiResp.Choices) == 0 {
		return CompletionResponse{}, fmt.Errorf("openai: 응답에 choices가 없음")
	}

	choice := apiResp.Choices[0]
	usage := TokenUsage{
		PromptTokens:     apiResp.Usage.PromptTokens,
		CompletionTokens: apiResp.Usage.CompletionTokens,
		TotalTokens:      apiResp.Usage.TotalTokens,
		CalledAt:         calledAt,
		RequestID:        apiResp.ID,
	}
	slog.Info("llm token usage",
		"request_id", usage.RequestID,
		"prompt_tokens", usage.PromptTokens,
		"completion_tokens", usage.CompletionTokens,
		"total_tokens", usage.TotalTokens,
		"called_at", usage.CalledAt.Format(time.RFC3339),
	)
	return CompletionResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
		Usage:        usage,
	}, nil
}
