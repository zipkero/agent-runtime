package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/message"
)

// ollamaWireToolCall 은 /api/chat 메시지 내 tool_calls[] 원소다.
// Ollama가 id를 반환하지 않을 수 있으므로 omitempty로 선언한다.
type ollamaWireToolCall struct {
	ID       string               `json:"id,omitempty"`
	Function ollamaWireToolCallFn `json:"function"`
}

type ollamaWireToolCallFn struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ollamaWireTool 은 요청 body의 tools[] 원소다.
type ollamaWireTool struct {
	Type     string             `json:"type"`
	Function ollamaWireToolSpec `json:"function"`
}

type ollamaWireToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

var _ LLMClient = (*OllamaClient)(nil)

// OllamaClient 는 net/http로 Ollama /api/chat을 직접 호출하는 LLMClient다.
type OllamaClient struct {
	host       string
	model      string
	httpClient *http.Client
}

// NewOllamaClient 는 config로 주입된 host와 model을 사용해 Ollama client를 생성한다.
// host 또는 model이 비어 있으면 error를 반환한다.
func NewOllamaClient(cfg config.Config) (*OllamaClient, error) {
	return newOllamaClient(cfg.Host, cfg.Model, http.DefaultClient)
}

// newOllamaClient 는 NewOllamaClient의 구현부이자 테스트 주입 지점이다.
// httpClient를 외부에서 받아 httptest 서버로 호출을 가로챌 수 있게 한다.
func newOllamaClient(host, model string, httpClient *http.Client) (*OllamaClient, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("%s is required for ollama client", config.EnvHost)
	}

	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("%s is required for ollama client", config.EnvModel)
	}

	return &OllamaClient{
		host:       host,
		model:      model,
		httpClient: httpClient,
	}, nil
}

// Chat 은 req를 Ollama /api/chat 한 번의 호출로 변환해 assistant 응답을 돌려준다.
// ctx는 HTTP 요청에 그대로 전파되어 취소·deadline을 따르며, ctx 취소는 context error로
// 표면화된다.
func (c *OllamaClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// ctx가 이미 취소된 경우 HTTP 호출 전에 먼저 반환한다.
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = c.model
	}

	wireMessages, err := ollamaMessagesFromInternal(req.Messages)
	if err != nil {
		return ChatResponse{}, err
	}

	wireTools, err := ollamaToolsFromInternal(req.Tools)
	if err != nil {
		return ChatResponse{}, err
	}

	body := ollamaChatRequest{
		Model:    model,
		Messages: wireMessages,
		Tools:    wireTools,
		Stream:   false,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.host+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// ctx 취소·deadline이 Do 에러에 가려지지 않도록 먼저 확인한다.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ChatResponse{}, ctxErr
		}
		return ChatResponse{}, fmt.Errorf("ollama http request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("ollama returned status %d", httpResp.StatusCode)
	}

	var wireResp ollamaChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&wireResp); err != nil {
		return ChatResponse{}, fmt.Errorf("decode ollama response: %w", err)
	}

	return ollamaResponseToInternal(wireResp)
}

// --- wire 타입 ---

// ollamaChatRequest 는 /api/chat 요청 body다.
type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaWireMessage `json:"messages"`
	Tools    []ollamaWireTool    `json:"tools,omitempty"`
	Stream   bool                `json:"stream"`
}

// ollamaWireMessage 는 Ollama /api/chat에서 주고받는 단일 메시지다.
// ToolCalls·ToolName·ToolCallID 는 tool 왕복 경로에서만 사용된다.
type ollamaWireMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content,omitempty"`
	ToolCalls  []ollamaWireToolCall `json:"tool_calls,omitempty"`
	ToolName   string               `json:"tool_name,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

// ollamaChatResponse 는 /api/chat 단일(stream:false) 응답 body다.
type ollamaChatResponse struct {
	Message ollamaWireMessage `json:"message"`
}

// --- 변환 함수 ---

// ollamaMessagesFromInternal 은 internal 메시지 슬라이스를 Ollama wire 메시지로 변환한다.
// - assistant 메시지: text 블록은 content로, tool_call 블록은 tool_calls[]로 사상한다.
// - RoleTool 메시지: 각 tool_result 블록을 개별 wire 메시지로 1:N 분리한다.
//   Ollama는 tool 결과를 하나씩 독립 메시지로 받으므로 내부적으로 하나의 RoleTool 메시지에
//   여러 tool_result가 담겨 있으면 각각 별도 메시지로 분리해야 한다.
func ollamaMessagesFromInternal(messages []message.Message) ([]ollamaWireMessage, error) {
	var wire []ollamaWireMessage

	for _, msg := range messages {
		role, err := ollamaRoleFromInternal(msg.Role)
		if err != nil {
			return nil, err
		}

		if msg.Role == message.RoleTool {
			// tool_result 블록 각각을 별도 wire 메시지로 1:N 변환한다.
			// Ollama는 tool 결과를 role:"tool" 개별 메시지로 받는다.
			expanded, err := ollamaToolResultMessages(msg.Content)
			if err != nil {
				return nil, err
			}
			wire = append(wire, expanded...)
			continue
		}

		// system·user·assistant 경로: text와 tool_call을 동일 메시지에 싣는다.
		var sb strings.Builder
		var toolCalls []ollamaWireToolCall

		for _, block := range msg.Content {
			switch block.Type {
			case message.BlockTypeText:
				sb.WriteString(block.Text)
			case message.BlockTypeToolCall:
				if block.ToolCall == nil {
					return nil, fmt.Errorf("tool_call block missing payload")
				}
				toolCalls = append(toolCalls, ollamaWireToolCall{
					ID: block.ToolCall.ID,
					Function: ollamaWireToolCallFn{
						Name:      block.ToolCall.Name,
						Arguments: block.ToolCall.Input,
					},
				})
			default:
				return nil, fmt.Errorf("unsupported content block type %q for role %q", block.Type, role)
			}
		}

		wire = append(wire, ollamaWireMessage{
			Role:      role,
			Content:   sb.String(),
			ToolCalls: toolCalls,
		})
	}

	return wire, nil
}

// ollamaToolResultMessages 는 tool_result 블록 슬라이스를 Ollama wire 메시지 슬라이스로
// 1:N 변환한다. 각 tool_result는 독립 메시지 {role:"tool", content, tool_call_id}로
// 변환되며, tool_call_id는 internal ToolResult.ToolCallID를 그대로 싣는다.
// internal ToolResult에 tool 이름 필드가 없어 tool_name은 싣지 않으며, 호출-결과 매칭은
// tool_call_id로만 이뤄진다.
func ollamaToolResultMessages(blocks []message.ContentBlock) ([]ollamaWireMessage, error) {
	var msgs []ollamaWireMessage
	for _, block := range blocks {
		if block.Type != message.BlockTypeToolResult {
			return nil, fmt.Errorf("expected tool_result block, got %q", block.Type)
		}
		if block.ToolResult == nil {
			return nil, fmt.Errorf("tool_result block missing payload")
		}
		msgs = append(msgs, ollamaWireMessage{
			Role:       "tool",
			Content:    block.ToolResult.Content,
			ToolCallID: block.ToolResult.ToolCallID,
		})
	}
	return msgs, nil
}

// ollamaRoleFromInternal 은 internal Role을 Ollama wire role 문자열로 변환한다.
func ollamaRoleFromInternal(role message.Role) (string, error) {
	switch role {
	case message.RoleSystem:
		return "system", nil
	case message.RoleUser:
		return "user", nil
	case message.RoleAssistant:
		return "assistant", nil
	case message.RoleTool:
		return "tool", nil
	default:
		return "", fmt.Errorf("unsupported message role %q", role)
	}
}

// ollamaResponseToInternal 은 Ollama 응답을 assistant 역할의 internal ChatResponse로 변환한다.
// text와 tool_call은 동시에 존재할 수 있으므로 둘 다 처리한다.
// Ollama가 tool_call id를 비워 보내면 응답 내 등장 순번 기반 결정적 ID(call_<n>)를 채운다.
// 순번 기반 ID는 요청 변환 때 그대로 wire로 되실리므로 왕복 동안 동일 ID가 유지된다.
func ollamaResponseToInternal(resp ollamaChatResponse) (ChatResponse, error) {
	var blocks []message.ContentBlock

	// text가 있으면 text 블록을 먼저 추가한다.
	if resp.Message.Content != "" {
		blocks = append(blocks, message.NewTextBlock(resp.Message.Content))
	}

	// tool_call 블록 변환: id가 비면 등장 순번(0-based)으로 결정적 ID를 부여한다.
	for i, tc := range resp.Message.ToolCalls {
		id := tc.ID
		if strings.TrimSpace(id) == "" {
			// Ollama id 부재 시 순번 기반 결정적 ID를 채워 internal ID로 싣는다.
			// 이 ID는 이후 요청 시 tool_calls[].id·tool 메시지 tool_call_id로 되실려
			// 왕복 동안 동일 ID를 유지한다(analysis.md §5 D4).
			id = fmt.Sprintf("call_%d", i)
		}
		blocks = append(blocks, message.NewToolCallBlock(message.ToolCall{
			ID:    id,
			Name:  tc.Function.Name,
			Input: tc.Function.Arguments,
		}))
	}

	if len(blocks) == 0 {
		return ChatResponse{}, fmt.Errorf("ollama response contains no content or tool_calls")
	}

	return ChatResponse{
		Message: message.Message{
			Role:    message.RoleAssistant,
			Content: blocks,
		},
	}, nil
}

// ollamaToolsFromInternal 은 internal ToolSpec 슬라이스를 Ollama wire tools[]로 변환한다.
// InputSchema의 type/properties/required를 분리해 parameters에 사상하고, 나머지 키도 보존한다.
// claude.go claudeInputSchemaFromInternal의 패턴을 참조하되 Ollama wire 형태(map)로 맞춘다.
func ollamaToolsFromInternal(tools []message.ToolSpec) ([]ollamaWireTool, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	wire := make([]ollamaWireTool, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, fmt.Errorf("tool name is required")
		}

		params, err := ollamaParametersFromSchema(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q input schema: %w", tool.Name, err)
		}

		wire = append(wire, ollamaWireTool{
			Type: "function",
			Function: ollamaWireToolSpec{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  params,
			},
		})
	}
	return wire, nil
}

// ollamaParametersFromSchema 는 JSON Schema RawMessage를 Ollama parameters map으로 변환한다.
// type/properties/required는 top-level 키로 분리하고, 나머지 키도 그대로 보존한다.
// claude.go claudeInputSchemaFromInternal과 달리 SDK 구조체가 아닌 map[string]any를 반환한다.
func ollamaParametersFromSchema(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}

	// type이 있으면 object여야 한다.
	if value, ok := schema["type"].(string); ok && value != "object" {
		return nil, fmt.Errorf("expected object schema, got %q", value)
	}

	// 모든 키를 그대로 두고 map을 반환한다(type·properties·required 포함).
	// Ollama는 parameters를 JSON Schema 그대로 받으므로 추출·재조립이 필요 없다.
	return schema, nil
}
