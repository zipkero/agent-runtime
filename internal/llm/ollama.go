package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// ollamaChatRequest 는 내부 메시지를 Ollama Chat API wire format으로 옮긴 요청이다.
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

// NewOllamaClient 는 Ollama Chat API를 LLMClient contract 뒤에 연결한다.
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

func decodeOllamaResponse(resp ollamaChatResponse) ChatResponse {
	toolCalls := make([]message.ToolCall, 0, len(resp.Message.ToolCalls))
	for _, call := range resp.Message.ToolCalls {
		args := call.Function.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		toolCalls = append(toolCalls, message.ToolCall{
			Name:      call.Function.Name,
			Arguments: args,
		})
	}

	return ChatResponse{
		Provider:   ProviderOllama,
		Model:      resp.Model,
		Message:    message.Assistant(resp.Message.Content, toolCalls...),
		StopReason: resp.DoneReason,
		Usage: Usage{
			InputTokens:  resp.PromptEvalCount,
			OutputTokens: resp.EvalCount,
			TotalTokens:  resp.PromptEvalCount + resp.EvalCount,
		},
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
