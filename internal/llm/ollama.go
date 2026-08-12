package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

const (
	ollamaChatPath          = "/api/chat"
	ollamaRequestMediaType  = "application/json"
	ollamaProviderOperation = "chat"
)

type ollamaClient struct {
	httpClient *http.Client
	host       string
	model      string
}

// ollamaChatRequest 구조체는 내부 메시지를 Ollama Chat API 전송 형식으로 옮긴 요청이다.
type ollamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []ollamaRequestMessage `json:"messages"`
	Stream   bool                   `json:"stream"`
	Tools    []ollamaTool           `json:"tools,omitempty"`
}

type ollamaTool struct {
	Type     string               `json:"type"`
	Function ollamaToolDefinition `json:"function"`
}

type ollamaToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ollamaRequestMessage struct {
	Role           string           `json:"role"`
	Content        string           `json:"content"`
	ToolResultName string           `json:"tool_name,omitempty"`
	ToolCalls      []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ollamaChatResponse struct {
	Model           string                `json:"model"`
	Message         ollamaResponseMessage `json:"message"`
	Done            bool                  `json:"done"`
	DoneReason      string                `json:"done_reason"`
	PromptEvalCount int                   `json:"prompt_eval_count"`
	EvalCount       int                   `json:"eval_count"`
}

type ollamaResponseMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls"`
}

type ollamaErrorResponse struct {
	Error string `json:"error"`
}

// NewOllamaClient 함수는 Ollama Chat API를 LLMClient 계약 뒤에 연결한다.
func NewOllamaClient(cfg ProviderConfig) (LLMClient, error) {
	return newOllamaClient(cfg, http.DefaultClient)
}

func newOllamaClient(cfg ProviderConfig, httpClient *http.Client) (LLMClient, error) {
	normalized := cfg
	normalized.Provider = string(ProviderOllama)
	if err := validateRequired(normalized, ProviderRequirements{Model: true, Host: true}); err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, configError(ProviderOllama, "create client", errors.New("http client is required"))
	}
	host := strings.TrimRight(strings.TrimSpace(cfg.Host), "/")
	if host == "" {
		return nil, configError(ProviderOllama, "create client", errors.New("host is required"))
	}

	return &ollamaClient{
		httpClient: httpClient,
		host:       host,
		model:      strings.TrimSpace(cfg.Model),
	}, nil
}

func (c *ollamaClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	body, err := c.buildRequest(req)
	if err != nil {
		return ChatResponse{}, err
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, providerError(ProviderOllama, ollamaProviderOperation, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+ollamaChatPath, bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, providerError(ProviderOllama, ollamaProviderOperation, err)
	}
	httpReq.Header.Set("Content-Type", ollamaRequestMediaType)
	httpReq.Header.Set("Accept", ollamaRequestMediaType)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isTimeoutError(ctx, err) {
			return ChatResponse{}, timeoutError(ProviderOllama, ollamaProviderOperation, err)
		}
		return ChatResponse{}, providerError(ProviderOllama, ollamaProviderOperation, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return ChatResponse{}, readOllamaHTTPError(httpResp)
	}

	var decoded ollamaChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return ChatResponse{}, providerError(ProviderOllama, ollamaProviderOperation, err)
	}

	return decodeOllamaResponse(decoded), nil
}

// StreamChat 메서드는 Ollama Chat API의 NDJSON stream을 provider 중립 text delta와 완성 응답으로 변환한다.
// HTTP status 오류는 body를 읽기 전에 기존 Chat과 같은 오류로 반환하고, 이후 decode 오류, done:true 없는 EOF는
// provider 오류로 종료한다. Consumer가 순회를 중단하면 그 시점에서 정리되고 추가 event를 만들지 않는다.
func (c *ollamaClient) StreamChat(ctx context.Context, req ChatRequest) iter.Seq2[ChatStreamEvent, error] {
	return func(yield func(ChatStreamEvent, error) bool) {
		body, err := c.buildRequest(req)
		if err != nil {
			yield(ChatStreamEvent{}, err)
			return
		}
		body.Stream = true

		payload, err := json.Marshal(body)
		if err != nil {
			yield(ChatStreamEvent{}, providerError(ProviderOllama, ollamaProviderOperation, err))
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+ollamaChatPath, bytes.NewReader(payload))
		if err != nil {
			yield(ChatStreamEvent{}, providerError(ProviderOllama, ollamaProviderOperation, err))
			return
		}
		httpReq.Header.Set("Content-Type", ollamaRequestMediaType)
		httpReq.Header.Set("Accept", ollamaRequestMediaType)

		httpResp, err := c.httpClient.Do(httpReq)
		if err != nil {
			if isTimeoutError(ctx, err) {
				yield(ChatStreamEvent{}, timeoutError(ProviderOllama, ollamaProviderOperation, err))
				return
			}
			yield(ChatStreamEvent{}, providerError(ProviderOllama, ollamaProviderOperation, err))
			return
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
			yield(ChatStreamEvent{}, readOllamaHTTPError(httpResp))
			return
		}

		acc := &ollamaStreamAccumulator{}
		decoder := json.NewDecoder(httpResp.Body)
		for {
			var chunk ollamaChatResponse
			if err := decoder.Decode(&chunk); err != nil {
				if isTimeoutError(ctx, err) {
					yield(ChatStreamEvent{}, timeoutError(ProviderOllama, ollamaProviderOperation, err))
					return
				}
				if errors.Is(err, io.EOF) {
					yield(ChatStreamEvent{}, providerError(ProviderOllama, ollamaProviderOperation, errors.New("stream ended before done chunk")))
					return
				}
				yield(ChatStreamEvent{}, providerError(ProviderOllama, ollamaProviderOperation, err))
				return
			}

			delta, resp, done := acc.handle(chunk)
			if delta != "" {
				if !yield(ChatStreamEvent{Kind: ChatStreamEventTextDelta, TextDelta: delta}, nil) {
					return
				}
			}
			if done {
				yield(ChatStreamEvent{Kind: ChatStreamEventResponse, Response: resp}, nil)
				return
			}
		}
	}
}

// ollamaStreamAccumulator 구조체는 NDJSON chunk를 도착 순서대로 받아 완성된 ChatResponse로 조립한다.
// Content는 chunk마다 이어 붙이고 Tool call은 chunk가 실어 온 순서대로 누적하며, done:true 전에는 완성
// 응답을 만들지 않는다.
type ollamaStreamAccumulator struct {
	model      string
	content    strings.Builder
	toolCalls  []message.ToolCall
	doneReason string
	promptEval int
	evalCount  int
}

// handle 메서드는 NDJSON chunk 하나를 반영하고, 새 text delta와 done:true에서 조립한 완성 응답·완료 여부를
// 반환한다.
func (a *ollamaStreamAccumulator) handle(chunk ollamaChatResponse) (delta string, resp *ChatResponse, done bool) {
	if chunk.Model != "" {
		a.model = chunk.Model
	}
	if chunk.Message.Content != "" {
		a.content.WriteString(chunk.Message.Content)
		delta = chunk.Message.Content
	}
	a.toolCalls = append(a.toolCalls, ollamaResponseToolCalls(chunk.Message.ToolCalls)...)

	if !chunk.Done {
		return delta, nil, false
	}

	a.doneReason = chunk.DoneReason
	a.promptEval = chunk.PromptEvalCount
	a.evalCount = chunk.EvalCount

	resp = &ChatResponse{
		Provider:     ProviderOllama,
		Model:        a.model,
		Message:      message.Assistant(a.content.String(), a.toolCalls...),
		FinishReason: normalizeOllamaFinishReason(a.doneReason, len(a.toolCalls) > 0),
		StopReason:   a.doneReason,
		Usage: Usage{
			InputTokens:  a.promptEval,
			OutputTokens: a.evalCount,
			TotalTokens:  a.promptEval + a.evalCount,
		},
	}
	return delta, resp, true
}

func (c *ollamaClient) buildRequest(req ChatRequest) (ollamaChatRequest, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = c.model
	}
	if model == "" {
		return ollamaChatRequest{}, configError(ProviderOllama, "build request", errors.New("model is required"))
	}

	messages := make([]ollamaRequestMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		switch msg.Role {
		case message.RoleSystem, message.RoleUser, message.RoleAssistant:
			messages = append(messages, ollamaRequestMessage{
				Role:      string(msg.Role),
				Content:   msg.Text,
				ToolCalls: ollamaToolCalls(msg.ToolCalls),
			})
		case message.RoleTool:
			messages = append(messages, ollamaToolResultMessage(msg))
		default:
			return ollamaChatRequest{}, configError(ProviderOllama, "build request", fmt.Errorf("unsupported message role %q", msg.Role))
		}
	}

	return ollamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
		Tools:    ollamaTools(req.Tools),
	}, nil
}

func ollamaTools(schemas []message.ToolSchema) []ollamaTool {
	if len(schemas) == 0 {
		return nil
	}

	tools := make([]ollamaTool, 0, len(schemas))
	for _, schema := range schemas {
		parameters := schema.InputSchema
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{}`)
		}
		tools = append(tools, ollamaTool{
			Type: "function",
			Function: ollamaToolDefinition{
				Name:        schema.Name,
				Description: schema.Description,
				Parameters:  parameters,
			},
		})
	}
	return tools
}

func ollamaToolCalls(calls []message.ToolCall) []ollamaToolCall {
	if len(calls) == 0 {
		return nil
	}

	toolCalls := make([]ollamaToolCall, 0, len(calls))
	for _, call := range calls {
		args := call.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		toolCalls = append(toolCalls, ollamaToolCall{
			Function: ollamaToolFunction{
				Name:      call.Name,
				Arguments: args,
			},
		})
	}
	return toolCalls
}

// ollamaToolResultMessage 함수는 Tool 결과를 Ollama가 받는 형태로 옮긴다.
// Chat API에는 tool call ID 필드가 없어 결과를 tool_name으로만 이어붙이므로 ToolCallID는 전달하지 않는다.
func ollamaToolResultMessage(msg message.Message) ollamaRequestMessage {
	result := msg.ToolResult
	if result == nil {
		return ollamaRequestMessage{Role: string(message.RoleTool)}
	}
	return ollamaRequestMessage{
		Role:           string(message.RoleTool),
		Content:        result.Content,
		ToolResultName: result.Name,
	}
}

// ollamaResponseToolCalls 함수는 Ollama 응답의 Tool call을 Runtime 내부 표현으로 옮긴다.
// Chat API 응답에는 호출 ID가 없어 ID를 비워 두고 이름만 보존한다.
func ollamaResponseToolCalls(calls []ollamaToolCall) []message.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	toolCalls := make([]message.ToolCall, 0, len(calls))
	for _, call := range calls {
		args := call.Function.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		toolCalls = append(toolCalls, message.ToolCall{
			Name:      call.Function.Name,
			Arguments: args,
		})
	}
	return toolCalls
}

func decodeOllamaResponse(resp ollamaChatResponse) ChatResponse {
	toolCalls := ollamaResponseToolCalls(resp.Message.ToolCalls)

	return ChatResponse{
		Provider:     ProviderOllama,
		Model:        resp.Model,
		Message:      message.Assistant(resp.Message.Content, toolCalls...),
		FinishReason: normalizeOllamaFinishReason(resp.DoneReason, len(toolCalls) > 0),
		StopReason:   resp.DoneReason,
		Usage: Usage{
			InputTokens:  resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
			TotalTokens:  resp.PromptEvalCount + resp.EvalCount,
		},
	}
}

func normalizeOllamaFinishReason(doneReason string, hasToolCalls bool) FinishReason {
	switch strings.TrimSpace(doneReason) {
	case "stop":
		if hasToolCalls {
			return FinishReasonToolCall
		}
		return FinishReasonComplete
	case "length":
		return FinishReasonLengthLimit
	default:
		return FinishReasonUnknown
	}
}

func readOllamaHTTPError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return providerError(ProviderOllama, ollamaProviderOperation, fmt.Errorf("http %d: read error body: %w", resp.StatusCode, err))
	}

	var decoded ollamaErrorResponse
	if err := json.Unmarshal(body, &decoded); err == nil && decoded.Error != "" {
		return providerError(ProviderOllama, ollamaProviderOperation, fmt.Errorf("http %d: %s", resp.StatusCode, decoded.Error))
	}

	return providerError(ProviderOllama, ollamaProviderOperation, fmt.Errorf("http %d: provider returned error", resp.StatusCode))
}
