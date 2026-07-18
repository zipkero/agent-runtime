package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFileUsesDefaultsWithoutEnvFile(t *testing.T) {
	clearConfigEnv(t)

	cfg, err := LoadFile(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if cfg.LLMProvider != DefaultLLMProvider {
		t.Fatalf("LLMProvider = %q, want %q", cfg.LLMProvider, DefaultLLMProvider)
	}
	if cfg.LLMHost != DefaultLLMHost {
		t.Fatalf("LLMHost = %q, want %q", cfg.LLMHost, DefaultLLMHost)
	}
	if cfg.LLMTimeout != DefaultLLMTimeout {
		t.Fatalf("LLMTimeout = %s, want %s", cfg.LLMTimeout, DefaultLLMTimeout)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
	}
}

func TestLoadFileReadsDotEnv(t *testing.T) {
	clearConfigEnv(t)
	path := writeEnvFile(t, `
# local settings
LLM_PROVIDER=claude
LLM_MODEL=claude-sonnet-4-5
LLM_HOST=https://api.anthropic.com
LLM_API_KEY=file-secret
LLM_TIMEOUT=1500ms
TAVILY_API_KEY=tavily-secret
LOG_LEVEL=debug
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if cfg.LLMProvider != "claude" {
		t.Fatalf("LLMProvider = %q, want claude", cfg.LLMProvider)
	}
	if cfg.LLMModel != "claude-sonnet-4-5" {
		t.Fatalf("LLMModel = %q, want claude-sonnet-4-5", cfg.LLMModel)
	}
	if cfg.LLMHost != "https://api.anthropic.com" {
		t.Fatalf("LLMHost = %q, want https://api.anthropic.com", cfg.LLMHost)
	}
	if cfg.LLMAPIKey != "file-secret" {
		t.Fatalf("LLMAPIKey = %q, want file-secret", cfg.LLMAPIKey)
	}
	if cfg.LLMTimeout != 1500*time.Millisecond {
		t.Fatalf("LLMTimeout = %s, want 1500ms", cfg.LLMTimeout)
	}
	if cfg.TavilyAPIKey != "tavily-secret" {
		t.Fatalf("TavilyAPIKey = %q, want tavily-secret", cfg.TavilyAPIKey)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadFilePrefersProcessEnvOverDotEnv(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv(envLLMProvider, "ollama")
	t.Setenv(envLLMModel, "env-model")
	t.Setenv(envLLMTimeout, "45s")

	path := writeEnvFile(t, `
LLM_PROVIDER=claude
LLM_MODEL=file-model
LLM_API_KEY=file-secret
LLM_TIMEOUT=10s
`)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if cfg.LLMProvider != "ollama" {
		t.Fatalf("LLMProvider = %q, want ollama", cfg.LLMProvider)
	}
	if cfg.LLMModel != "env-model" {
		t.Fatalf("LLMModel = %q, want env-model", cfg.LLMModel)
	}
	if cfg.LLMAPIKey != "file-secret" {
		t.Fatalf("LLMAPIKey = %q, want file-secret", cfg.LLMAPIKey)
	}
	if cfg.LLMTimeout != 45*time.Second {
		t.Fatalf("LLMTimeout = %s, want 45s", cfg.LLMTimeout)
	}
}

func TestLoadFileRejectsInvalidDuration(t *testing.T) {
	clearConfigEnv(t)
	path := writeEnvFile(t, "LLM_TIMEOUT=soon\n")

	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want error")
	}
	if !strings.Contains(err.Error(), envLLMTimeout) {
		t.Fatalf("LoadFile() error = %q, want %s context", err, envLLMTimeout)
	}
}

func TestLoadFileValidatesPositiveLLMTimeout(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		process   bool
		want      time.Duration
		wantError bool
	}{
		{name: "dot env positive", value: "1ns", want: time.Nanosecond},
		{name: "dot env zero", value: "0s", wantError: true},
		{name: "dot env negative", value: "-1ns", wantError: true},
		{name: "process env positive", value: "2s", process: true, want: 2 * time.Second},
		{name: "process env zero", value: "0s", process: true, wantError: true},
		{name: "process env negative", value: "-2s", process: true, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			path := writeEnvFile(t, "LLM_TIMEOUT=3s\n")
			if tt.process {
				t.Setenv(envLLMTimeout, tt.value)
			} else {
				path = writeEnvFile(t, "LLM_TIMEOUT="+tt.value+"\n")
			}

			cfg, err := LoadFile(path)
			if tt.wantError {
				if err == nil {
					t.Fatal("LoadFile() error = nil, want non-positive timeout error")
				}
				if !strings.Contains(err.Error(), envLLMTimeout) {
					t.Fatalf("LoadFile() error = %q, want %s context", err, envLLMTimeout)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFile() error = %v", err)
			}
			if cfg.LLMTimeout != tt.want {
				t.Fatalf("LLMTimeout = %s, want %s", cfg.LLMTimeout, tt.want)
			}
		})
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		envLLMProvider,
		envLLMModel,
		envLLMHost,
		envLLMAPIKey,
		envLLMTimeout,
		envTavilyKey,
		envLogLevel,
	} {
		key := key
		value, ok := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q) error = %v", key, err)
		}
		t.Cleanup(func() {
			if ok {
				if err := os.Setenv(key, value); err != nil {
					t.Fatalf("Setenv(%q) cleanup error = %v", key, err)
				}
				return
			}
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("Unsetenv(%q) cleanup error = %v", key, err)
			}
		})
	}
}

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
