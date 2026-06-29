package llm

import (
	"context"
	"testing"

	"github.com/zipkero/agent-runtime/internal/message"
)

type stubClient struct {
	response ChatResponse
}

func (c stubClient) Chat(context.Context, ChatRequest) (ChatResponse, error) {
	return c.response, nil
}

// TestChatResponsePreservesAssistantToolCalls 는 ChatResponse가 provider 응답에서 온 assistant tool call을 내부 메시지에 보존하는지 확인한다.
func TestChatResponsePreservesAssistantToolCalls(t *testing.T) {
	resp := ChatResponse{
		Provider: ProviderClaude,
		Model:    "claude-test",
		Message: message.Assistant("tool requested", message.ToolCall{
			ID:   "call-1",
			Name: "search",
		}),
		StopReason: "tool_use",
	}

	if resp.Provider != ProviderClaude || resp.Model != "claude-test" {
		t.Fatalf("response provider/model = %q/%q, want claude/claude-test", resp.Provider, resp.Model)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.Message.ToolCalls))
	}
	if resp.Message.ToolCalls[0].Name != "search" {
		t.Fatalf("ToolCall name = %q, want search", resp.Message.ToolCalls[0].Name)
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("StopReason = %q, want tool_use", resp.StopReason)
	}
}

// TestRegistrySelectsRegisteredProvider 는 Registry가 provider 이름을 정규화하고 등록된 factory로 client를 만드는지 확인한다.
func TestRegistrySelectsRegisteredProvider(t *testing.T) {
	registry := NewRegistry()
	wantClient := stubClient{}
	if err := registry.Register(ProviderClaude, ProviderRequirements{Model: true, APIKey: true}, func(cfg ProviderConfig) (LLMClient, error) {
		if cfg.Provider != string(ProviderClaude) {
			t.Fatalf("Provider = %q, want %q", cfg.Provider, ProviderClaude)
		}
		if cfg.Model != "claude-test" || cfg.APIKey != "secret" {
			t.Fatalf("ProviderConfig = %+v, want model and api key", cfg)
		}
		return wantClient, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	client, err := registry.NewClient(ProviderConfig{
		Provider: " Claude ",
		Model:    "claude-test",
		APIKey:   "secret",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() client = nil, want value")
	}
}

// TestRegistryRejectsUnsupportedProvider 는 Registry가 알 수 없는 provider를 설정 오류로 거절하는지 확인한다.
func TestRegistryRejectsUnsupportedProvider(t *testing.T) {
	_, err := NewRegistry().NewClient(ProviderConfig{Provider: "gpt"})
	if err == nil {
		t.Fatal("NewClient() error = nil, want unsupported provider error")
	}
	if !IsKind(err, ErrorKindConfig) {
		t.Fatalf("NewClient() error kind mismatch: %v", err)
	}
}

// TestRegistryValidatesRequiredProviderConfig 는 Registry가 provider factory를 호출하기 전에 필수 설정 누락을 거절하는지 확인한다.
func TestRegistryValidatesRequiredProviderConfig(t *testing.T) {
	tests := []struct {
		name         string
		requirements ProviderRequirements
		cfg          ProviderConfig
	}{
		{
			name:         "model",
			requirements: ProviderRequirements{Model: true},
			cfg:          ProviderConfig{Provider: string(ProviderOllama)},
		},
		{
			name:         "host",
			requirements: ProviderRequirements{Host: true},
			cfg:          ProviderConfig{Provider: string(ProviderOllama)},
		},
		{
			name:         "api key",
			requirements: ProviderRequirements{APIKey: true},
			cfg:          ProviderConfig{Provider: string(ProviderClaude)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			if err := registry.Register(normalizeProvider(tt.cfg.Provider), tt.requirements, func(ProviderConfig) (LLMClient, error) {
				t.Fatal("factory should not be called when required config is missing")
				return nil, nil
			}); err != nil {
				t.Fatalf("Register() error = %v", err)
			}

			_, err := registry.NewClient(tt.cfg)
			if err == nil {
				t.Fatal("NewClient() error = nil, want config error")
			}
			if !IsKind(err, ErrorKindConfig) {
				t.Fatalf("NewClient() error kind mismatch: %v", err)
			}
		})
	}
}
