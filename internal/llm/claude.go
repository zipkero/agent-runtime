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

// claudeMessageRequest 는 Claude Messages API에 직접 보내는 JSON wire format이다.
type claudeMessageRequest struct {
	Model     string                 `json:"model"`
	MaxTokens int                    `json:"max_tokens"`
	System    string                 `json:"system,omitempty"`
	Messages  []claudeRequestMessage `json:"messages"`
}

type claudeRequestMessage struct {
	Role    string                      `json:"role"`
	Content []claudeRequestContentBlock `json:"content"`
}

// claudeRequestContentBlock 은 내부 text/tool 표현을 Claude content block으로 옮기는 provider 경계 타입이다.
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

// NewClaudeClient 는 Claude Messages API를 LLMClient contract 뒤에 연결한다.
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

// buildRequest 는 Runtime 메시지를 Claude가 받는 system 필드와 대화 message 배열로 분리한다.
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
	}, nil
}

// claudeContentBlocks 는 Phase 1 범위에서 text, tool_use, tool_result 표현만 provider 형식으로 보존한다.
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

// decodeClaudeResponse 는 Claude content block 중 Runtime이 이후 단계에서 재사용할 text와 tool_use만 내부 응답으로 옮긴다.
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
		Provider:   ProviderClaude,
		Model:      resp.Model,
		Message:    message.Assistant(text.String(), toolCalls...),
		StopReason: resp.StopReason,
		Usage: Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

// readHTTPError 는 provider 오류 메시지를 보존하되 API key가 섞인 경우 외부 오류에서 제거한다.
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

// isTimeoutError 는 context deadline과 net/http timeout을 같은 LLM timeout 오류로 분류한다.
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
