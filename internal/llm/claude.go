package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"strings"

	"github.com/zipkero/agent-runtime/internal/message"
)

const (
	claudeDefaultEndpoint = "https://api.anthropic.com"
	claudeMessagesPath    = "/v1/messages"
	claudeAPIVersion      = "2023-06-01"
	// Messages API는 max_tokens를 필수로 요구하므로 호출자가 지정하지 않아도 이 값을 채워 보낸다.
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
	Stream    bool                   `json:"stream,omitempty"`
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

// StreamChat 메서드는 Claude Messages API의 SSE stream을 provider 중립 text delta와 완성 응답으로 변환한다.
// HTTP status 오류는 body를 읽기 전에 기존 Chat과 같은 오류로 반환하고, 이후 SSE error event, decode 실패,
// 잘못된 Tool JSON과 message_stop 없는 EOF는 provider 오류로 종료한다. Consumer가 순회를 중단하면 그 시점에서
// 정리되고 추가 event를 만들지 않는다.
func (c *claudeClient) StreamChat(ctx context.Context, req ChatRequest) iter.Seq2[ChatStreamEvent, error] {
	return func(yield func(ChatStreamEvent, error) bool) {
		body, err := c.buildRequest(req)
		if err != nil {
			yield(ChatStreamEvent{}, err)
			return
		}
		body.Stream = true

		payload, err := json.Marshal(body)
		if err != nil {
			yield(ChatStreamEvent{}, providerError(ProviderClaude, claudeProviderOperation, err))
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+claudeMessagesPath, bytes.NewReader(payload))
		if err != nil {
			yield(ChatStreamEvent{}, providerError(ProviderClaude, claudeProviderOperation, err))
			return
		}
		httpReq.Header.Set("Content-Type", claudeRequestMediaType)
		httpReq.Header.Set("Accept", "text/event-stream")
		httpReq.Header.Set("x-api-key", c.apiKey)
		httpReq.Header.Set("anthropic-version", claudeAPIVersion)

		httpResp, err := c.httpClient.Do(httpReq)
		if err != nil {
			if isTimeoutError(ctx, err) {
				yield(ChatStreamEvent{}, timeoutError(ProviderClaude, claudeProviderOperation, err))
				return
			}
			yield(ChatStreamEvent{}, providerError(ProviderClaude, claudeProviderOperation, err))
			return
		}
		defer httpResp.Body.Close()

		if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
			yield(ChatStreamEvent{}, c.readHTTPError(httpResp))
			return
		}

		acc := &claudeStreamAccumulator{}
		reader := bufio.NewReader(httpResp.Body)
		for {
			data, readErr := readSSEEventData(reader)
			if readErr != nil {
				if isTimeoutError(ctx, readErr) {
					yield(ChatStreamEvent{}, timeoutError(ProviderClaude, claudeProviderOperation, readErr))
					return
				}
				yield(ChatStreamEvent{}, providerError(ProviderClaude, claudeProviderOperation, fmt.Errorf("stream ended before message_stop: %w", readErr)))
				return
			}

			delta, resp, done, err := acc.handle(data)
			if err != nil {
				yield(ChatStreamEvent{}, providerError(ProviderClaude, claudeProviderOperation, err))
				return
			}
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

// readSSEEventData 함수는 SSE event 하나를 읽어 data 필드를 합친 문자열로 반환한다.
// Multi-line data는 개행으로 이어 붙이고, event·id 같은 다른 필드와 comment(ping) 줄은 무시한다.
func readSSEEventData(r *bufio.Reader) (string, error) {
	var dataLines []string
	for {
		line, readErr := r.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed != "" {
			if strings.HasPrefix(trimmed, "data:") {
				dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(trimmed, "data:"), " "))
			}
			// event, id와 comment(:로 시작하는 ping) 줄은 data 조립에 쓰지 않는다.
		} else if len(line) > 0 && readErr == nil {
			// blank line은 event 경계다.
			return strings.Join(dataLines, "\n"), nil
		}
		if readErr != nil {
			return strings.Join(dataLines, "\n"), readErr
		}
	}
}

// claudeStreamEnvelope 구조체는 SSE data 필드에 담긴 Claude stream event의 공통 JSON 형태다.
type claudeStreamEnvelope struct {
	Type         string                      `json:"type"`
	Index        int                         `json:"index"`
	Message      *claudeStreamMessage        `json:"message,omitempty"`
	ContentBlock *claudeResponseContentBlock `json:"content_block,omitempty"`
	Delta        *claudeStreamDelta          `json:"delta,omitempty"`
	Usage        *claudeUsage                `json:"usage,omitempty"`
	Error        *claudeStreamError          `json:"error,omitempty"`
}

type claudeStreamMessage struct {
	Model string      `json:"model"`
	Usage claudeUsage `json:"usage"`
}

// claudeStreamDelta 구조체는 content_block_delta와 message_delta event의 delta 필드를 함께 담는다.
type claudeStreamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
	StopReason  string `json:"stop_reason"`
}

type claudeStreamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// claudeStreamBlockKind 타입은 stream content block이 text인지 tool_use인지 구분한다.
type claudeStreamBlockKind int

const (
	claudeStreamBlockText claudeStreamBlockKind = iota
	claudeStreamBlockToolUse
)

// claudeStreamBlock 구조체는 진행 중인 content block 하나의 조립 상태를 보관한다.
// closed는 content_block_stop을 받았는지를 나타내며, message_stop 시점에 열린 block이 남아 있으면
// 불완전 content block으로 취급해 성공 응답을 만들지 않는다.
type claudeStreamBlock struct {
	kind     claudeStreamBlockKind
	id       string
	name     string
	closed   bool
	text     strings.Builder
	partial  strings.Builder
	toolCall *message.ToolCall
}

// claudeStreamAccumulator 구조체는 SSE event를 순서대로 받아 완성된 ChatResponse로 조립한다.
// Tool call은 content_block_stop에서 부분 JSON을 완성한 뒤에만 만들어지므로 실행 경계로 부분 값이 새지 않는다.
type claudeStreamAccumulator struct {
	model      string
	blocks     map[int]*claudeStreamBlock
	order      []int
	stopReason string
	inputTok   int
	outputTok  int
}

// handle 메서드는 SSE event 하나의 JSON data를 반영하고, 새 text delta, message_stop에서 조립한 완성 응답과
// 완료 여부를 반환한다. Ping과 알 수 없는 event type은 무시한다.
func (a *claudeStreamAccumulator) handle(data string) (delta string, resp *ChatResponse, done bool, err error) {
	if strings.TrimSpace(data) == "" {
		return "", nil, false, nil
	}

	var env claudeStreamEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		return "", nil, false, fmt.Errorf("decode stream event: %w", err)
	}

	switch env.Type {
	case "message_start":
		if env.Message != nil {
			a.model = env.Message.Model
			a.inputTok = env.Message.Usage.InputTokens
		}
	case "content_block_start":
		if env.ContentBlock == nil {
			return "", nil, false, fmt.Errorf("content_block_start missing content_block at index %d", env.Index)
		}
		if err := a.startBlock(env.Index, *env.ContentBlock); err != nil {
			return "", nil, false, err
		}
	case "content_block_delta":
		if env.Delta == nil {
			return "", nil, false, fmt.Errorf("content_block_delta missing delta at index %d", env.Index)
		}
		block, ok := a.blocks[env.Index]
		if !ok {
			return "", nil, false, fmt.Errorf("content_block_delta before content_block_start at index %d", env.Index)
		}
		switch env.Delta.Type {
		case "text_delta":
			if block.kind != claudeStreamBlockText {
				return "", nil, false, fmt.Errorf("text_delta for non-text content block at index %d", env.Index)
			}
			block.text.WriteString(env.Delta.Text)
			delta = env.Delta.Text
		case "input_json_delta":
			if block.kind != claudeStreamBlockToolUse {
				return "", nil, false, fmt.Errorf("input_json_delta for non-tool_use content block at index %d", env.Index)
			}
			block.partial.WriteString(env.Delta.PartialJSON)
		default:
			// 문서화되지 않은 delta type은 무시한다.
		}
	case "content_block_stop":
		if err := a.finishBlock(env.Index); err != nil {
			return "", nil, false, err
		}
	case "message_delta":
		if env.Delta != nil && env.Delta.StopReason != "" {
			a.stopReason = env.Delta.StopReason
		}
		if env.Usage != nil {
			a.outputTok = env.Usage.OutputTokens
		}
	case "message_stop":
		if index, open := a.openBlockIndex(); open {
			return "", nil, false, fmt.Errorf("message_stop before content_block_stop at index %d", index)
		}
		resp = a.buildResponse()
		done = true
	case "error":
		msg := "provider stream error"
		if env.Error != nil && env.Error.Message != "" {
			msg = env.Error.Message
		}
		return "", nil, false, errors.New(msg)
	default:
		// ping과 알 수 없는 event type은 무시한다.
	}

	return delta, resp, done, nil
}

// startBlock 메서드는 새 content block을 등록한다. 이미 등록된 index(닫힌 block 포함)가 다시 오면 order의
// index 유일성이 깨지고 그 block의 text·Tool JSON이 덮어써지므로, 등록하지 않고 잘못된 순서로 오류를 반환한다.
func (a *claudeStreamAccumulator) startBlock(index int, cb claudeResponseContentBlock) error {
	if _, exists := a.blocks[index]; exists {
		return fmt.Errorf("duplicate content_block_start at index %d", index)
	}

	block := &claudeStreamBlock{id: cb.ID, name: cb.Name}
	if cb.Type == "tool_use" {
		block.kind = claudeStreamBlockToolUse
	} else if cb.Text != "" {
		block.text.WriteString(cb.Text)
	}
	if a.blocks == nil {
		a.blocks = make(map[int]*claudeStreamBlock)
	}
	a.blocks[index] = block
	a.order = append(a.order, index)
	return nil
}

// openBlockIndex 메서드는 content_block_stop을 받지 못한 채 열려 있는 content block이 있는지 확인한다.
// message_stop이 이런 block보다 먼저 오면 불완전 content block을 성공 응답으로 조립하지 않기 위해 쓰인다.
func (a *claudeStreamAccumulator) openBlockIndex() (index int, open bool) {
	for _, idx := range a.order {
		if block := a.blocks[idx]; block != nil && !block.closed {
			return idx, true
		}
	}
	return 0, false
}

// finishBlock 메서드는 content block을 닫고, tool_use라면 모은 부분 JSON을 하나의 문서로 확정한다.
// content_block_start 없이 도착한 index나 유효한 JSON을 이루지 못한 Tool call은 stream을 종료시킬 오류로 반환한다.
func (a *claudeStreamAccumulator) finishBlock(index int) error {
	block, ok := a.blocks[index]
	if !ok {
		return fmt.Errorf("content_block_stop before content_block_start at index %d", index)
	}
	block.closed = true
	if block.kind != claudeStreamBlockToolUse {
		return nil
	}

	raw := block.partial.String()
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	if !json.Valid([]byte(raw)) {
		return fmt.Errorf("invalid tool_use input json at content block %d", index)
	}
	block.toolCall = &message.ToolCall{
		ID:        block.id,
		Name:      block.name,
		Arguments: json.RawMessage(raw),
	}
	return nil
}

// buildResponse 메서드는 지금까지 모은 content block과 완료 metadata를 non-streaming 응답과 같은 형태로 조립한다.
func (a *claudeStreamAccumulator) buildResponse() *ChatResponse {
	var text strings.Builder
	var toolCalls []message.ToolCall
	for _, index := range a.order {
		block := a.blocks[index]
		if block == nil {
			continue
		}
		switch block.kind {
		case claudeStreamBlockText:
			text.WriteString(block.text.String())
		case claudeStreamBlockToolUse:
			if block.toolCall != nil {
				toolCalls = append(toolCalls, *block.toolCall)
			}
		}
	}

	return &ChatResponse{
		Provider:     ProviderClaude,
		Model:        a.model,
		Message:      message.Assistant(text.String(), toolCalls...),
		FinishReason: normalizeClaudeFinishReason(a.stopReason),
		StopReason:   a.stopReason,
		Usage: Usage{
			InputTokens:  a.inputTok,
			OutputTokens: a.outputTok,
			TotalTokens:  a.inputTok + a.outputTok,
		},
	}
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
			// Messages API에는 tool 역할이 없고 tool_result block을 user 메시지로 받는다.
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
