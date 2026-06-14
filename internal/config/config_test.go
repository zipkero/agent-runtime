package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zipkero/agent-runtime/internal/config"
)

// writeDotEnv 는 임시 디렉터리에 .env를 만들고 작업 디렉터리를 그 곳으로 바꾼다.
// t.Chdir·t.TempDir 가 테스트 종료 시 원복·정리를 맡는다.
func writeDotEnv(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Chdir(dir)
}

func TestLoadReadsFromDotEnvFile(t *testing.T) {
	writeDotEnv(t, "ANTHROPIC_API_KEY=sk-from-dotenv\nANTHROPIC_MODEL=claude-dotenv\nTAVILY_API_KEY=tvly-from-dotenv\n")

	// 실제 환경변수는 비워, .env 값이 채워지는지 본다.
	t.Setenv(config.EnvAnthropicAPIKey, "placeholder")
	os.Unsetenv(config.EnvAnthropicAPIKey)
	t.Setenv(config.EnvModel, "placeholder")
	os.Unsetenv(config.EnvModel)
	t.Setenv(config.EnvTavilyAPIKey, "placeholder")
	os.Unsetenv(config.EnvTavilyAPIKey)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.AnthropicAPIKey != "sk-from-dotenv" {
		t.Errorf("expected API key from .env %q, got %q", "sk-from-dotenv", cfg.AnthropicAPIKey)
	}
	if cfg.Model != "claude-dotenv" {
		t.Errorf("expected model from .env %q, got %q", "claude-dotenv", cfg.Model)
	}
	if cfg.TavilyAPIKey != "tvly-from-dotenv" {
		t.Errorf("expected Tavily API key from .env %q, got %q", "tvly-from-dotenv", cfg.TavilyAPIKey)
	}
}

func TestRealEnvTakesPrecedenceOverDotEnv(t *testing.T) {
	writeDotEnv(t, "ANTHROPIC_API_KEY=sk-from-dotenv\n")

	// 실제 환경변수가 .env보다 우선해야 한다. model은 필수라 실제 환경변수로 채운다.
	t.Setenv(config.EnvAnthropicAPIKey, "sk-from-realenv")
	t.Setenv(config.EnvModel, "claude-realenv")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.AnthropicAPIKey != "sk-from-realenv" {
		t.Errorf("expected real env to win with %q, got %q", "sk-from-realenv", cfg.AnthropicAPIKey)
	}
}

func TestLoadAppliesDefaultTimeout(t *testing.T) {
	t.Setenv(config.EnvAnthropicAPIKey, "sk-ant-test")
	t.Setenv(config.EnvModel, "claude-test-model")
	t.Setenv(config.EnvTimeout, "")
	t.Setenv(config.EnvTavilyAPIKey, "tvly-test")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.AnthropicAPIKey != "sk-ant-test" {
		t.Errorf("expected API key %q, got %q", "sk-ant-test", cfg.AnthropicAPIKey)
	}
	if cfg.Timeout != config.DefaultTimeout {
		t.Errorf("expected default timeout %s, got %s", config.DefaultTimeout, cfg.Timeout)
	}
	if cfg.TavilyAPIKey != "tvly-test" {
		t.Errorf("expected Tavily API key %q, got %q", "tvly-test", cfg.TavilyAPIKey)
	}
}

func TestLoadAllowsMissingTavilyAPIKey(t *testing.T) {
	t.Setenv(config.EnvAnthropicAPIKey, "sk-ant-test")
	t.Setenv(config.EnvModel, "claude-test-model")
	t.Setenv(config.EnvTimeout, "")
	t.Setenv(config.EnvTavilyAPIKey, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.TavilyAPIKey != "" {
		t.Errorf("expected empty Tavily API key, got %q", cfg.TavilyAPIKey)
	}
}

func TestLoadRequiresAPIKey(t *testing.T) {
	t.Setenv(config.EnvAnthropicAPIKey, "")
	t.Setenv(config.EnvModel, "claude-test-model")
	t.Setenv(config.EnvTimeout, "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected Load to return an error when API key is missing")
	}
}

func TestLoadRequiresModel(t *testing.T) {
	t.Setenv(config.EnvAnthropicAPIKey, "sk-ant-test")
	t.Setenv(config.EnvModel, "")
	t.Setenv(config.EnvTimeout, "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected Load to return an error when model is missing")
	}
}

func TestLoadReadsModelFromEnv(t *testing.T) {
	t.Setenv(config.EnvAnthropicAPIKey, "sk-ant-test")
	t.Setenv(config.EnvModel, "claude-test-model")
	t.Setenv(config.EnvTimeout, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Model != "claude-test-model" {
		t.Errorf("expected model %q, got %q", "claude-test-model", cfg.Model)
	}
}

func TestLoadReflectsDifferentEnvironmentValues(t *testing.T) {
	t.Setenv(config.EnvAnthropicAPIKey, "sk-ant-one")
	t.Setenv(config.EnvModel, "claude-one")
	t.Setenv(config.EnvTimeout, "2s")

	first, err := config.Load()
	if err != nil {
		t.Fatalf("first Load returned error: %v", err)
	}

	t.Setenv(config.EnvAnthropicAPIKey, "sk-ant-two")
	t.Setenv(config.EnvModel, "claude-two")
	t.Setenv(config.EnvTimeout, "5s")

	second, err := config.Load()
	if err != nil {
		t.Fatalf("second Load returned error: %v", err)
	}

	if first == second {
		t.Fatal("expected different config values from different environment variables")
	}
	if second.AnthropicAPIKey != "sk-ant-two" {
		t.Errorf("expected second API key %q, got %q", "sk-ant-two", second.AnthropicAPIKey)
	}
	if second.Model != "claude-two" {
		t.Errorf("expected second model %q, got %q", "claude-two", second.Model)
	}
	if second.Timeout != 5*time.Second {
		t.Errorf("expected second timeout %s, got %s", 5*time.Second, second.Timeout)
	}
}

func TestLoadAppliesTimeoutOverride(t *testing.T) {
	t.Setenv(config.EnvAnthropicAPIKey, "sk-ant-test")
	t.Setenv(config.EnvModel, "claude-test-model")
	t.Setenv(config.EnvTimeout, "1500ms")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Timeout != 1500*time.Millisecond {
		t.Errorf("expected timeout override %s, got %s", 1500*time.Millisecond, cfg.Timeout)
	}
}

func TestLoadRejectsInvalidTimeout(t *testing.T) {
	t.Setenv(config.EnvAnthropicAPIKey, "sk-ant-test")
	t.Setenv(config.EnvModel, "claude-test-model")
	t.Setenv(config.EnvTimeout, "not-a-duration")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected Load to return an error for invalid timeout")
	}
}
