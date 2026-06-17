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

	body := ollamaChatRequest{
		Model:    model,
		Messages: wireMessages,
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
	Stream   bool                `json:"stream"`
}

// ollamaWireMessage 는 Ollama /api/chat에서 주고받는 단일 메시지다.
type ollamaWireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChatResponse 는 /api/chat 단일(stream:false) 응답 body다.
type ollamaChatResponse struct {
	Message ollamaWireMessage `json:"message"`
}

// --- 변환 함수 ---

// ollamaMessagesFromInternal 은 internal 메시지 슬라이스를 Ollama wire 메시지로 변환한다.
// text 블록만 처리하며, tool_call·tool_result 블록은 task-003에서 확장된다.
// 빈 content 문자열을 허용하므로 text 블록이 없는 메시지는 content=""로 사상한다.
func ollamaMessagesFromInternal(messages []message.Message) ([]ollamaWireMessage, error) {
	var wire []ollamaWireMessage

	for _, msg := range messages {
		role, err := ollamaRoleFromInternal(msg.Role)
		if err != nil {
			return nil, err
		}

		// text 블록들을 이어 붙여 content로 만든다.
		// tool_call·tool_result 블록은 이번 Task 범위(text 경로)에서는 건너뛰되
		// task-003에서 이 분기를 채울 수 있도록 switch로 구조를 열어 둔다.
		var sb strings.Builder
		for _, block := range msg.Content {
			switch block.Type {
			case message.BlockTypeText:
				sb.WriteString(block.Text)
			case message.BlockTypeToolCall, message.BlockTypeToolResult:
				// task-003에서 변환 로직을 추가한다.
			}
		}

		wire = append(wire, ollamaWireMessage{
			Role:    role,
			Content: sb.String(),
		})
	}

	return wire, nil
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
func ollamaResponseToInternal(resp ollamaChatResponse) (ChatResponse, error) {
	block := message.NewTextBlock(resp.Message.Content)

	return ChatResponse{
		Message: message.Message{
			Role:    message.RoleAssistant,
			Content: []message.ContentBlock{block},
		},
	}, nil
}
