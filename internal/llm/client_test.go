package llm

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/zipkero/agent-runtime/internal/config"
	"github.com/zipkero/agent-runtime/internal/message"
)

func TestNewClientSelectsProvider(t *testing.T) {
	cases := []struct {
		name     string
		cfg      config.Config
		wantType any
	}{
		{
			name: "ollama provider",
			cfg: config.Config{
				Provider: config.ProviderOllama,
				Host:     "http://localhost:11434",
				Model:    "llama3",
			},
			wantType: &OllamaClient{},
		},
		{
			name: "claude provider",
			cfg: config.Config{
				Provider: config.ProviderClaude,
				APIKey:   "test-key",
				Model:    "claude-test",
			},
			wantType: &ClaudeClient{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient(tc.cfg)
			if err != nil {
				t.Fatalf("NewClient returned error: %v", err)
			}

			switch tc.wantType.(type) {
			case *OllamaClient:
				if _, ok := client.(*OllamaClient); !ok {
					t.Fatalf("expected *OllamaClient, got %T", client)
				}
			case *ClaudeClient:
				if _, ok := client.(*ClaudeClient); !ok {
					t.Fatalf("expected *ClaudeClient, got %T", client)
				}
			default:
				t.Fatalf("unsupported test type %T", tc.wantType)
			}
		})
	}
}

func TestNewClientRejectsUnsupportedProvider(t *testing.T) {
	_, err := NewClient(config.Config{
		Provider: config.Provider("openai"),
		Model:    "test-model",
	})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestLLMStreamerContractDeliversStreamEvents(t *testing.T) {
	// Streaming contract는 provider 구현 세부를 숨기고 text delta와 완료 message를 순서대로 노출해야 한다.
	wantComplete := message.Message{
		Role: message.RoleAssistant,
		Content: []message.ContentBlock{
			message.NewTextBlock("hello world"),
		},
	}
	client := &stubStreamingClient{
		stream: &stubChatStream{
			events: []ChatStreamEvent{
				{
					Type:      ChatStreamEventTypeTextDelta,
					TextDelta: "hello ",
				},
				{
					Type:      ChatStreamEventTypeTextDelta,
					TextDelta: "world",
				},
				{
					Type:    ChatStreamEventTypeMessageComplete,
					Message: wantComplete,
				},
			},
		},
	}

	stream, err := client.Stream(context.Background(), ChatRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv first event returned error: %v", err)
	}
	if first.Type != ChatStreamEventTypeTextDelta || first.TextDelta != "hello " {
		t.Fatalf("first event = %#v, want text delta hello", first)
	}

	second, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv second event returned error: %v", err)
	}
	if second.Type != ChatStreamEventTypeTextDelta || second.TextDelta != "world" {
		t.Fatalf("second event = %#v, want text delta world", second)
	}

	complete, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv complete event returned error: %v", err)
	}
	if complete.Type != ChatStreamEventTypeMessageComplete {
		t.Fatalf("complete event type = %q, want %q", complete.Type, ChatStreamEventTypeMessageComplete)
	}
	if complete.Message.Role != wantComplete.Role || complete.Message.Content[0].Text != "hello world" {
		t.Fatalf("complete message = %#v, want %#v", complete.Message, wantComplete)
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !client.stream.closed {
		t.Fatal("expected Close to mark stream closed")
	}
}

func TestLLMStreamerContractReturnsRecvError(t *testing.T) {
	// Recv error는 별도 event type이 아니라 stream reader의 error로 호출자에게 전파되어야 한다.
	wantErr := errors.New("stream failed")
	client := &stubStreamingClient{
		stream: &stubChatStream{
			events: []ChatStreamEvent{
				{
					Type:      ChatStreamEventTypeTextDelta,
					TextDelta: "partial",
				},
			},
			err: wantErr,
		},
	}

	stream, err := client.Stream(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv first event returned error: %v", err)
	}
	_, err = stream.Recv()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Recv error = %v, want %v", err, wantErr)
	}
}

type stubStreamingClient struct {
	stream *stubChatStream
}

func (c *stubStreamingClient) Stream(context.Context, ChatRequest) (ChatStream, error) {
	return c.stream, nil
}

type stubChatStream struct {
	events []ChatStreamEvent
	err    error
	closed bool
}

func (s *stubChatStream) Recv() (ChatStreamEvent, error) {
	if len(s.events) == 0 {
		if s.err != nil {
			return ChatStreamEvent{}, s.err
		}
		return ChatStreamEvent{}, io.EOF
	}

	event := s.events[0]
	s.events = s.events[1:]
	return event, nil
}

func (s *stubChatStream) Close() error {
	s.closed = true
	return nil
}
