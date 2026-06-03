package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/message"
)

const defaultClaudeMaxTokens int64 = 1024

var _ LLMClient = (*ClaudeClient)(nil)

// ClaudeClient 는 공식 Anthropic SDK로 Claude Messages API를 호출하는 LLMClient다.
type ClaudeClient struct {
	client    anthropic.Client
	model     string
	maxTokens int64
}

type claudeClientOptions struct {
	baseURL    string
	httpClient *http.Client
	maxTokens  int64
}

// NewClaudeClient 는 config로 주입된 api key와 model을 사용해 Claude client를 생성한다.
func NewClaudeClient(cfg config.Config) (*ClaudeClient, error) {
	return newClaudeClient(cfg, claudeClientOptions{})
}

func newClaudeClient(cfg config.Config, opts claudeClientOptions) (*ClaudeClient, error) {
	apiKey := strings.TrimSpace(cfg.AnthropicAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s is required", config.EnvAnthropicAPIKey)
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("%s is required", config.EnvModel)
	}

	sdkOptions := []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(0),
	}
	if opts.baseURL != "" {
		sdkOptions = append(sdkOptions, option.WithBaseURL(opts.baseURL))
	}
	if opts.httpClient != nil {
		sdkOptions = append(sdkOptions, option.WithHTTPClient(opts.httpClient))
	}

	maxTokens := opts.maxTokens
	if maxTokens <= 0 {
		maxTokens = defaultClaudeMaxTokens
	}

	return &ClaudeClient{
		client:    anthropic.NewClient(sdkOptions...),
		model:     model,
		maxTokens: maxTokens,
	}, nil
}

func (c *ClaudeClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}

	params, err := c.newMessageParams(req)
	if err != nil {
		return ChatResponse{}, err
	}

	resp, err := c.client.Messages.New(ctx, params)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ChatResponse{}, ctxErr
		}
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized {
			return ChatResponse{}, fmt.Errorf("claude authentication failed: %w", err)
		}
		return ChatResponse{}, err
	}

	return claudeMessageToChatResponse(resp)
}

func (c *ClaudeClient) newMessageParams(req ChatRequest) (anthropic.MessageNewParams, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = c.model
	}

	messages, system, err := claudeMessagesFromInternal(req.Messages)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}

	tools, err := claudeToolsFromInternal(req.Tools)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}

	return anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: c.maxTokens,
		Messages:  messages,
		System:    system,
		Tools:     tools,
	}, nil
}

func claudeMessagesFromInternal(messages []message.Message) ([]anthropic.MessageParam, []anthropic.TextBlockParam, error) {
	var params []anthropic.MessageParam
	var system []anthropic.TextBlockParam

	for _, msg := range messages {
		if msg.Role == message.RoleSystem {
			blocks, err := claudeSystemBlocksFromInternal(msg.Content)
			if err != nil {
				return nil, nil, err
			}
			system = append(system, blocks...)
			continue
		}

		blocks, err := claudeContentBlocksFromInternal(msg.Content)
		if err != nil {
			return nil, nil, err
		}

		switch msg.Role {
		case message.RoleUser:
			params = append(params, anthropic.NewUserMessage(blocks...))
		case message.RoleAssistant:
			params = append(params, anthropic.NewAssistantMessage(blocks...))
		case message.RoleTool:
			params = append(params, anthropic.NewUserMessage(blocks...))
		default:
			return nil, nil, fmt.Errorf("unsupported message role %q", msg.Role)
		}
	}

	return params, system, nil
}

func claudeSystemBlocksFromInternal(blocks []message.ContentBlock) ([]anthropic.TextBlockParam, error) {
	var system []anthropic.TextBlockParam
	for _, block := range blocks {
		if block.Type != message.BlockTypeText {
			return nil, fmt.Errorf("system message supports only text blocks, got %q", block.Type)
		}
		system = append(system, anthropic.TextBlockParam{Text: block.Text})
	}
	return system, nil
}

func claudeContentBlocksFromInternal(blocks []message.ContentBlock) ([]anthropic.ContentBlockParamUnion, error) {
	var params []anthropic.ContentBlockParamUnion
	for _, block := range blocks {
		switch block.Type {
		case message.BlockTypeText:
			params = append(params, anthropic.NewTextBlock(block.Text))
		case message.BlockTypeToolCall:
			if block.ToolCall == nil {
				return nil, fmt.Errorf("tool_call block missing payload")
			}
			params = append(params, anthropic.NewToolUseBlock(
				block.ToolCall.ID,
				block.ToolCall.Input,
				block.ToolCall.Name,
			))
		case message.BlockTypeToolResult:
			if block.ToolResult == nil {
				return nil, fmt.Errorf("tool_result block missing payload")
			}
			params = append(params, anthropic.NewToolResultBlock(
				block.ToolResult.ToolCallID,
				block.ToolResult.Content,
				block.ToolResult.IsError,
			))
		default:
			return nil, fmt.Errorf("unsupported content block type %q", block.Type)
		}
	}
	return params, nil
}

func claudeToolsFromInternal(tools []message.ToolSpec) ([]anthropic.ToolUnionParam, error) {
	var params []anthropic.ToolUnionParam
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("tool name is required")
		}

		schema, err := claudeInputSchemaFromInternal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q input schema: %w", tool.Name, err)
		}

		param := anthropic.ToolUnionParamOfTool(schema, tool.Name)
		if tool.Description != "" {
			param.OfTool.Description = anthropic.String(tool.Description)
		}
		params = append(params, param)
	}
	return params, nil
}

func claudeInputSchemaFromInternal(raw json.RawMessage) (anthropic.ToolInputSchemaParam, error) {
	if len(raw) == 0 {
		return anthropic.ToolInputSchemaParam{}, nil
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return anthropic.ToolInputSchemaParam{}, err
	}

	if value, ok := schema["type"].(string); ok && value != "object" {
		return anthropic.ToolInputSchemaParam{}, fmt.Errorf("expected object schema, got %q", value)
	}

	properties := schema["properties"]
	required, err := requiredFields(schema["required"])
	if err != nil {
		return anthropic.ToolInputSchemaParam{}, err
	}

	delete(schema, "type")
	delete(schema, "properties")
	delete(schema, "required")

	return anthropic.ToolInputSchemaParam{
		Properties:  properties,
		Required:    required,
		ExtraFields: schema,
	}, nil
}

func requiredFields(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}

	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("required must be an array")
	}

	fields := make([]string, 0, len(values))
	for _, value := range values {
		field, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("required entries must be strings")
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func claudeMessageToChatResponse(resp *anthropic.Message) (ChatResponse, error) {
	if resp == nil {
		return ChatResponse{}, fmt.Errorf("claude response is nil")
	}

	blocks := make([]message.ContentBlock, 0, len(resp.Content))
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			blocks = append(blocks, message.NewTextBlock(block.Text))
		case "tool_use":
			blocks = append(blocks, message.NewToolCallBlock(message.ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			}))
		default:
			return ChatResponse{}, fmt.Errorf("unsupported claude response block type %q", block.Type)
		}
	}

	return ChatResponse{
		Message: message.Message{
			Role:    message.RoleAssistant,
			Content: blocks,
		},
	}, nil
}
