package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/message"
)

// TestNewOllamaClientRequiresHostAndModel 은 host 또는 model이 비면 생성자가 error를
// 반환하는지 검증한다.
func TestNewOllamaClientRequiresHostAndModel(t *testing.T) {
	cases := []struct {
		name  string
		cfg   config.Config
	}{
		{
			name: "host 부재",
			cfg:  config.Config{Model: "llama3"},
		},
		{
			name: "model 부재",
			cfg:  config.Config{Host: "http://localhost:11434"},
		},
		{
			name: "host와 model 모두 부재",
			cfg:  config.Config{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewOllamaClient(tc.cfg)
			if err == nil {
				t.Fatalf("expected error when %s, got nil", tc.name)
			}
		})
	}
}

// TestOllamaClientMapsRequestAndResponse 는 httptest로 /api/chat을 가로채
// 요청 body에 system·user 메시지와 stream:false·model이 실리는지,
// 응답 text가 assistant 메시지로 환원되는지 검증한다.
func TestOllamaClientMapsRequestAndResponse(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message": {
				"role": "assistant",
				"content": "안녕하세요, 무엇을 도와드릴까요?"
			}
		}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	resp, err := client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleSystem,
				Content: []message.ContentBlock{message.NewTextBlock("항상 한국어로 답변하라.")},
			},
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("안녕하세요.")},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	assertOllamaRequest(t, requestBody)
	assertOllamaResponse(t, resp)
}

// TestOllamaClientUsesRequestModelOverride 는 req.Model이 있으면 client 기본 model 대신
// 해당 값이 요청 body에 실리는지 검증한다.
func TestOllamaClientUsesRequestModelOverride(t *testing.T) {
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message": {"role": "assistant", "content": "ok"}}`))
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "default-model", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{
		Model: "override-model",
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("hello")},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	if requestBody["model"] != "override-model" {
		t.Fatalf("expected request model override, got %v", requestBody["model"])
	}
}

// TestOllamaClientReturnsContextErrorOnCancellation 은 취소된 ctx로 Chat을 호출하면
// context error가 그대로 표면화되는지 검증한다.
func TestOllamaClientReturnsContextErrorOnCancellation(t *testing.T) {
	client, err := newOllamaClient("http://localhost:11434", "llama3", http.DefaultClient)
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err = client.Chat(ctx, ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("hello")},
			},
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded error, got %v", err)
	}
}

// TestOllamaClientReturnsContextErrorOnCancellationDuringRequest 는 요청 중 ctx가 취소되면
// context error가 표면화되는지 검증한다.
func TestOllamaClientReturnsContextErrorOnCancellationDuringRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ctx가 취소될 때까지 기다려 in-flight 취소를 시뮬레이션한다.
		<-r.Context().Done()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 요청이 서버에 도달한 직후 취소되도록 별도 goroutine에서 cancel을 호출한다.
	go cancel()

	_, err = client.Chat(ctx, ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("hello")},
			},
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

// TestOllamaClientReturnsErrorOnNonOKStatus 는 서버가 비정상 status를 반환하면 error가
// 표면화되는지 검증한다.
func TestOllamaClientReturnsErrorOnNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := newOllamaClient(server.URL, "llama3", server.Client())
	if err != nil {
		t.Fatalf("newOllamaClient returned error: %v", err)
	}

	_, err = client.Chat(context.Background(), ChatRequest{
		Messages: []message.Message{
			{
				Role:    message.RoleUser,
				Content: []message.ContentBlock{message.NewTextBlock("hello")},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error on non-OK status, got nil")
	}
}

// assertOllamaRequest 는 가로챈 요청 body에 필수 필드가 올바르게 실렸는지 검증한다.
func assertOllamaRequest(t *testing.T, requestBody map[string]any) {
	t.Helper()

	// model 필드
	if requestBody["model"] != "llama3" {
		t.Fatalf("unexpected model: %v", requestBody["model"])
	}

	// stream:false
	if requestBody["stream"] != false {
		t.Fatalf("expected stream:false, got %v", requestBody["stream"])
	}

	// messages: system + user 두 개
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected messages: %#v", requestBody["messages"])
	}

	systemMsg, ok := messages[0].(map[string]any)
	if !ok || systemMsg["role"] != "system" {
		t.Fatalf("unexpected system message: %#v", messages[0])
	}
	if systemMsg["content"] != "항상 한국어로 답변하라." {
		t.Fatalf("unexpected system message content: %v", systemMsg["content"])
	}

	userMsg, ok := messages[1].(map[string]any)
	if !ok || userMsg["role"] != "user" {
		t.Fatalf("unexpected user message: %#v", messages[1])
	}
	if userMsg["content"] != "안녕하세요." {
		t.Fatalf("unexpected user message content: %v", userMsg["content"])
	}
}

// assertOllamaResponse 는 응답이 assistant 역할의 text 블록으로 환원됐는지 검증한다.
func assertOllamaResponse(t *testing.T, resp ChatResponse) {
	t.Helper()

	if resp.Message.Role != message.RoleAssistant {
		t.Fatalf("expected assistant role, got %q", resp.Message.Role)
	}
	if len(resp.Message.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Message.Content))
	}
	block := resp.Message.Content[0]
	if block.Type != message.BlockTypeText {
		t.Fatalf("expected text block, got %q", block.Type)
	}
	if block.Text != "안녕하세요, 무엇을 도와드릴까요?" {
		t.Fatalf("unexpected response text: %q", block.Text)
	}
}
