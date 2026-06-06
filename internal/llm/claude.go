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

// Messages API는 max_tokens가 필수라 config에 값이 없을 때 적용할 기본값을 둔다.
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

// newClaudeClient 은 NewClaudeClient의 구현부이자 테스트 주입 지점이다.
// opts로 baseURL·httpClient를 받아 httptest 서버로 SDK 호출을 가로챌 수 있게 한다.
func newClaudeClient(cfg config.Config, opts claudeClientOptions) (*ClaudeClient, error) {
	apiKey := strings.TrimSpace(cfg.AnthropicAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("%s is required", config.EnvAnthropicAPIKey)
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("%s is required", config.EnvModel)
	}

	// SDK 환경변수 기본값을 끄고 인증을 config로만 주입한다. 재시도를 0으로 둬
	// 일시 실패(429·5xx 등)나 timeout이 자동 재시도·백오프 대기에 가려지지 않고
	// 호출 1회 결과 그대로 에러로 표면화되게 한다.
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

// Chat 은 req를 Claude Messages API 한 번의 호출로 변환해 assistant 응답을 돌려준다.
// ctx는 SDK 호출에 그대로 전파되어 취소·deadline을 따르며, ctx 취소·timeout과
// 인증 실패(401)는 호출자가 구분할 수 있는 error로 표면화된다.
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
		// SDK가 ctx 취소·deadline을 자체 에러로 감싸므로 원래 context 에러로 되돌린다.
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

// claudeMessagesFromInternal 은 internal 메시지를 Claude messages로 변환하되,
// system 역할은 Claude API 규약상 별도 System 필드로 분리해 두 번째 값으로 반환한다.
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
			// tool_result는 Anthropic wire에서 user 메시지의 content block으로 전달된다.
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

// claudeInputSchemaFromInternal 은 tool의 JSON schema를 SDK ToolInputSchemaParam으로
// 변환한다. SDK는 properties·required를 분리 필드로 받고 나머지 키는 ExtraFields로
// 넘기므로, 추출한 키는 원본 map에서 제거해 중복 전달을 막는다.
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

// claudeMessageToChatResponse 는 Claude 응답을 assistant 역할의 internal 메시지로
// 되돌린다. text·tool_use 블록만 처리하며 그 외 블록 타입은 에러로 반환한다.
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
