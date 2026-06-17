package llm

import (
	"testing"

	"github.com/zipkero/agent-runtime/internal/config"
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
