package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

const (
	claudeDefaultEndpoint   = "https://api.anthropic.com"
	claudeMessagesPath      = "/v1/messages"
	claudeAPIVersion        = "2023-06-01"
	claudeDefaultMaxTokens  = 1024
	claudeRequestMediaType  = "application/json"
	claudeProviderOperation = "chat"
)

type claudeClient struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	model      string
}

// claudeMessageRequest 구조체는 Claude Messages API에 직접 보내는 JSON 전송 형식이다.
type claudeMessageRequest struct {
	Model     string                 `json:"model"`
	MaxTokens int                    `json:"max_tokens"`
	System    string                 `json:"system,omitempty"`
	Messages  []claudeRequestMessage `json:"messages"`
	Tools     []claudeTool           `json:"tools,omitempty"`
}

type claudeTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type claudeRequestMessage struct {
	Role    string                      `json:"role"`
	Content []claudeRequestContentBlock `json:"content"`
}

// claudeRequestContentBlock 구조체는 내부 텍스트와 Tool 표현을 Claude content block으로 옮기는 공급자 경계 타입이다.
type claudeRequestContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type claudeMessageResponse struct {
	Model      string                       `json:"model"`
	Role       string                       `json:"role"`
	Content    []claudeResponseContentBlock `json:"content"`
	StopReason string                       `json:"stop_reason"`
	Usage      claudeUsage                  `json:"usage"`
}

type claudeResponseContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeErrorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// NewClaudeClient 함수는 Claude Messages API를 LLMClient 계약 뒤에 연결한다.
func NewClaudeClient(cfg ProviderConfig) (LLMClient, error) {
	return newClaudeClient(cfg, http.DefaultClient, claudeDefaultEndpoint)
}

func newClaudeClient(cfg ProviderConfig, httpClient *http.Client, endpoint string) (LLMClient, error) {
	normalized := cfg
	normalized.Provider = string(ProviderClaude)
	if err := validateRequired(normalized, ProviderRequirements{Model: true, APIKey: true}); err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, configError(ProviderClaude, "create client", errors.New("http client is required"))
	}
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return nil, configError(ProviderClaude, "create client", errors.New("endpoint is required"))
	}

	return &claudeClient{
		httpClient: httpClient,
		endpoint:   endpoint,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		model:      strings.TrimSpace(cfg.Model),
	}, nil
}

func (c *claudeClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	body, err := c.buildRequest(req)
	if err != nil {
		return ChatResponse{}, err
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, providerError(ProviderClaude, claudeProviderOperation, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+claudeMessagesPath, bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, providerError(ProviderClaude, claudeProviderOperation, err)
	}
	httpReq.Header.Set("Content-Type", claudeRequestMediaType)
	httpReq.Header.Set("Accept", claudeRequestMediaType)
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", claudeAPIVersion)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if isTimeoutError(ctx, err) {
			return ChatResponse{}, timeoutError(ProviderClaude, claudeProviderOperation, err)
		}
		return ChatResponse{}, providerError(ProviderClaude, claudeProviderOperation, err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		return ChatResponse{}, c.readHTTPError(httpResp)
	}

	var decoded claudeMessageResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return ChatResponse{}, providerError(ProviderClaude, claudeProviderOperation, err)
	}

	return decodeClaudeResponse(decoded), nil
}

// buildRequest 메서드는 Runtime 메시지를 Claude가 받는 system 필드와 대화 메시지 배열로 분리한다.
func (c *claudeClient) buildRequest(req ChatRequest) (claudeMessageRequest, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = c.model
	}
	if model == "" {
		return claudeMessageRequest{}, configError(ProviderClaude, "build request", errors.New("model is required"))
	}

	var systemParts []string
	var messages []claudeRequestMessage
	for _, msg := range req.Messages {
		switch msg.Role {
		case message.RoleSystem:
			if strings.TrimSpace(msg.Text) != "" {
				systemParts = append(systemParts, msg.Text)
			}
		case message.RoleUser, message.RoleAssistant:
			messages = append(messages, claudeRequestMessage{
				Role:    string(msg.Role),
				Content: claudeContentBlocks(msg),
			})
		case message.RoleTool:
			messages = append(messages, claudeRequestMessage{
				Role:    string(message.RoleUser),
				Content: claudeContentBlocks(msg),
			})
		default:
			return claudeMessageRequest{}, configError(ProviderClaude, "build request", fmt.Errorf("unsupported message role %q", msg.Role))
		}
	}

	return claudeMessageRequest{
		Model:     model,
		MaxTokens: claudeDefaultMaxTokens,
		System:    strings.Join(systemParts, "\n"),
		Messages:  messages,
		Tools:     claudeTools(req.Tools),
	}, nil
}

func claudeTools(schemas []message.ToolSchema) []claudeTool {
	if len(schemas) == 0 {
		return nil
	}

	tools := make([]claudeTool, 0, len(schemas))
	for _, schema := range schemas {
		inputSchema := schema.InputSchema
		if len(inputSchema) == 0 {
			inputSchema = json.RawMessage(`{}`)
		}
		tools = append(tools, claudeTool{
			Name:        schema.Name,
			Description: schema.Description,
			InputSchema: inputSchema,
		})
	}
	return tools
}

// claudeContentBlocks 함수는 Runtime의 text, tool_use, tool_result 표현을 Claude 형식으로 보존한다.
func claudeContentBlocks(msg message.Message) []claudeRequestContentBlock {
	var blocks []claudeRequestContentBlock
	if msg.Text != "" {
		blocks = append(blocks, claudeRequestContentBlock{
			Type: "text",
			Text: msg.Text,
		})
	}
	for _, call := range msg.ToolCalls {
		input := call.Arguments
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		blocks = append(blocks, claudeRequestContentBlock{
			Type:  "tool_use",
			ID:    call.ID,
			Name:  call.Name,
			Input: input,
		})
	}
	if msg.ToolResult != nil {
		blocks = append(blocks, claudeRequestContentBlock{
			Type:      "tool_result",
			ToolUseID: msg.ToolResult.ToolCallID,
			Content:   msg.ToolResult.Content,
			IsError:   msg.ToolResult.IsError,
		})
	}
	return blocks
}

// decodeClaudeResponse 함수는 Claude content block 중 Runtime이 재사용할 text와 tool_use만 내부 응답으로 옮긴다.
func decodeClaudeResponse(resp claudeMessageResponse) ChatResponse {
	var text strings.Builder
	var toolCalls []message.ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			args := block.Input
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			toolCalls = append(toolCalls, message.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: args,
			})
		}
	}

	return ChatResponse{
		Provider:     ProviderClaude,
		Model:        resp.Model,
		Message:      message.Assistant(text.String(), toolCalls...),
		FinishReason: normalizeClaudeFinishReason(resp.StopReason),
		StopReason:   resp.StopReason,
		Usage: Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

func normalizeClaudeFinishReason(stopReason string) FinishReason {
	switch strings.TrimSpace(stopReason) {
	case "end_turn", "stop_sequence":
		return FinishReasonComplete
	case "tool_use":
		return FinishReasonToolCall
	case "max_tokens":
		return FinishReasonLengthLimit
	case "refusal":
		return FinishReasonBlocked
	default:
		return FinishReasonUnknown
	}
}

// readHTTPError 메서드는 공급자 오류 메시지를 보존하되 API 키가 섞인 경우 외부 오류에서 제거한다.
func (c *claudeClient) readHTTPError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return providerError(ProviderClaude, claudeProviderOperation, fmt.Errorf("http %d: read error body: %w", resp.StatusCode, err))
	}

	var decoded claudeErrorResponse
	if err := json.Unmarshal(body, &decoded); err == nil && decoded.Error.Message != "" {
		msg := redactSecret(decoded.Error.Message, c.apiKey)
		if decoded.Error.Type != "" {
			msg = decoded.Error.Type + ": " + msg
		}
		return providerError(ProviderClaude, claudeProviderOperation, fmt.Errorf("http %d: %s", resp.StatusCode, msg))
	}

	return providerError(ProviderClaude, claudeProviderOperation, fmt.Errorf("http %d: provider returned error", resp.StatusCode))
}

func providerError(provider Provider, op string, err error) error {
	return &Error{
		Kind:     ErrorKindProvider,
		Provider: provider,
		Op:       op,
		Err:      err,
	}
}

func timeoutError(provider Provider, op string, err error) error {
	return &Error{
		Kind:     ErrorKindTimeout,
		Provider: provider,
		Op:       op,
		Err:      err,
	}
}

// isTimeoutError 함수는 context deadline과 net/http 제한 시간 초과를 같은 LLM 오류로 분류한다.
func isTimeoutError(ctx context.Context, err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func redactSecret(text string, secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return text
	}
	return strings.ReplaceAll(text, secret, "[redacted]")
}
