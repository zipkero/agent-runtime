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

// ---- provider 기본값·파싱 ----

// LLM_PROVIDER 미지정 시 provider가 ollama로 기본 설정된다.
func TestLoadDefaultsToOllamaWhenProviderUnset(t *testing.T) {
	t.Setenv(config.EnvProvider, "")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvTimeout, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Provider != config.ProviderOllama {
		t.Errorf("expected provider %q, got %q", config.ProviderOllama, cfg.Provider)
	}
}

// LLM_PROVIDER=ollama 명시 시 ollama provider로 로딩된다.
func TestLoadExplicitOllamaProvider(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvTimeout, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Provider != config.ProviderOllama {
		t.Errorf("expected provider %q, got %q", config.ProviderOllama, cfg.Provider)
	}
}

// LLM_PROVIDER=claude 명시 시 claude provider로 로딩된다.
func TestLoadExplicitClaudeProvider(t *testing.T) {
	t.Setenv(config.EnvProvider, "claude")
	t.Setenv(config.EnvModel, "claude-sonnet-4-5")
	t.Setenv(config.EnvAPIKey, "sk-test-key")
	t.Setenv(config.EnvTimeout, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Provider != config.ProviderClaude {
		t.Errorf("expected provider %q, got %q", config.ProviderClaude, cfg.Provider)
	}
}

// 미인식 provider 값은 error를 반환한다.
func TestLoadUnrecognizedProviderReturnsError(t *testing.T) {
	t.Setenv(config.EnvProvider, "openai")
	t.Setenv(config.EnvModel, "gpt-4")
	t.Setenv(config.EnvTimeout, "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected Load to return error for unrecognized provider")
	}
}

// ---- ollama provider 조건부 검증 ----

// ollama 선택 시 LLM_MODEL 부재는 error를 반환한다.
func TestLoadOllamaRequiresModel(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "")
	t.Setenv(config.EnvTimeout, "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected Load to return error when LLM_MODEL is missing for ollama")
	}
}

// ollama 선택 시 LLM_HOST 미지정이면 기본값이 채워진다.
func TestLoadOllamaUsesDefaultHostWhenUnset(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvHost, "")
	t.Setenv(config.EnvTimeout, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Host != config.DefaultOllamaHost {
		t.Errorf("expected default host %q, got %q", config.DefaultOllamaHost, cfg.Host)
	}
}

// ollama 선택 시 LLM_HOST가 지정되면 그 값이 채워진다.
func TestLoadOllamaReadsCustomHost(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvHost, "http://custom-host:11434")
	t.Setenv(config.EnvTimeout, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Host != "http://custom-host:11434" {
		t.Errorf("expected host %q, got %q", "http://custom-host:11434", cfg.Host)
	}
}

// ollama 선택 시 LLM_API_KEY 부재는 성공(미선택 provider 값 검증 안 함).
func TestLoadOllamaSucceedsWithoutAPIKey(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvAPIKey, "")
	t.Setenv(config.EnvTimeout, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.APIKey != "" {
		t.Errorf("expected empty APIKey, got %q", cfg.APIKey)
	}
}

// ---- claude provider 조건부 검증 ----

// claude 선택 시 LLM_API_KEY 부재는 error를 반환한다.
func TestLoadClaudeRequiresAPIKey(t *testing.T) {
	t.Setenv(config.EnvProvider, "claude")
	t.Setenv(config.EnvModel, "claude-sonnet-4-5")
	t.Setenv(config.EnvAPIKey, "")
	t.Setenv(config.EnvTimeout, "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected Load to return error when LLM_API_KEY is missing for claude")
	}
}

// claude 선택 시 LLM_MODEL 부재는 error를 반환한다.
func TestLoadClaudeRequiresModel(t *testing.T) {
	t.Setenv(config.EnvProvider, "claude")
	t.Setenv(config.EnvModel, "")
	t.Setenv(config.EnvAPIKey, "sk-test-key")
	t.Setenv(config.EnvTimeout, "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected Load to return error when LLM_MODEL is missing for claude")
	}
}

// claude 선택 시 LLM_HOST 부재는 성공(선택 항목).
func TestLoadClaudeSucceedsWithoutHost(t *testing.T) {
	t.Setenv(config.EnvProvider, "claude")
	t.Setenv(config.EnvModel, "claude-sonnet-4-5")
	t.Setenv(config.EnvAPIKey, "sk-test-key")
	t.Setenv(config.EnvHost, "")
	t.Setenv(config.EnvTimeout, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if cfg.Host != "" {
		t.Errorf("expected empty Host for claude without LLM_HOST, got %q", cfg.Host)
	}
}

// ---- timeout·공통 필드 ----

// LLM_TIMEOUT 미지정 시 기본 timeout이 채워진다.
func TestLoadAppliesDefaultTimeout(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvTimeout, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Timeout != config.DefaultTimeout {
		t.Errorf("expected default timeout %s, got %s", config.DefaultTimeout, cfg.Timeout)
	}
}

// LLM_TIMEOUT 재정의 시 그 값이 채워진다.
func TestLoadAppliesTimeoutOverride(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvTimeout, "1500ms")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Timeout != 1500*time.Millisecond {
		t.Errorf("expected timeout %s, got %s", 1500*time.Millisecond, cfg.Timeout)
	}
}

// 잘못된 LLM_TIMEOUT 형식은 error를 반환한다.
func TestLoadRejectsInvalidTimeout(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvTimeout, "not-a-duration")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected Load to return error for invalid timeout")
	}
}

// TAVILY_API_KEY 는 선택 항목이라 비어도 Load 는 성공한다.
func TestLoadAllowsMissingTavilyAPIKey(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvTimeout, "")
	t.Setenv(config.EnvTavilyAPIKey, "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.TavilyAPIKey != "" {
		t.Errorf("expected empty TavilyAPIKey, got %q", cfg.TavilyAPIKey)
	}
}

// TAVILY_API_KEY 가 지정되면 그 값이 채워진다.
func TestLoadReadsTavilyAPIKey(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "llama3-test")
	t.Setenv(config.EnvTimeout, "")
	t.Setenv(config.EnvTavilyAPIKey, "tvly-test-key")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.TavilyAPIKey != "tvly-test-key" {
		t.Errorf("expected TavilyAPIKey %q, got %q", "tvly-test-key", cfg.TavilyAPIKey)
	}
}

// ---- .env 파일 연동 ----

// .env 파일의 값이 환경변수로 로딩된다.
func TestLoadReadsFromDotEnvFile(t *testing.T) {
	writeDotEnv(t, "LLM_PROVIDER=ollama\nLLM_MODEL=llama3-dotenv\nLLM_HOST=http://dotenv-host:11434\nTAVILY_API_KEY=tvly-from-dotenv\n")

	// 실제 환경변수는 비워, .env 값이 채워지는지 본다.
	for _, key := range []string{config.EnvProvider, config.EnvModel, config.EnvHost, config.EnvTavilyAPIKey} {
		t.Setenv(key, "placeholder")
		os.Unsetenv(key)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Host != "http://dotenv-host:11434" {
		t.Errorf("expected Host from .env %q, got %q", "http://dotenv-host:11434", cfg.Host)
	}
	if cfg.Model != "llama3-dotenv" {
		t.Errorf("expected Model from .env %q, got %q", "llama3-dotenv", cfg.Model)
	}
	if cfg.TavilyAPIKey != "tvly-from-dotenv" {
		t.Errorf("expected TavilyAPIKey from .env %q, got %q", "tvly-from-dotenv", cfg.TavilyAPIKey)
	}
}

// 실제 환경변수가 .env보다 우선한다.
func TestRealEnvTakesPrecedenceOverDotEnv(t *testing.T) {
	writeDotEnv(t, "LLM_PROVIDER=ollama\nLLM_HOST=http://dotenv-host:11434\nLLM_MODEL=llama3-dotenv\n")

	t.Setenv(config.EnvHost, "http://realenv-host:11434")
	t.Setenv(config.EnvModel, "llama3-realenv")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Host != "http://realenv-host:11434" {
		t.Errorf("expected real env Host %q, got %q", "http://realenv-host:11434", cfg.Host)
	}
	if cfg.Model != "llama3-realenv" {
		t.Errorf("expected real env Model %q, got %q", "llama3-realenv", cfg.Model)
	}
}

// 다른 환경변수 값을 연속으로 Load 할 때 각기 다른 Config가 반환된다.
func TestLoadReflectsDifferentEnvironmentValues(t *testing.T) {
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvHost, "http://host-one:11434")
	t.Setenv(config.EnvModel, "llama3-one")
	t.Setenv(config.EnvTimeout, "2s")

	first, err := config.Load()
	if err != nil {
		t.Fatalf("first Load returned error: %v", err)
	}

	t.Setenv(config.EnvHost, "http://host-two:11434")
	t.Setenv(config.EnvModel, "llama3-two")
	t.Setenv(config.EnvTimeout, "5s")

	second, err := config.Load()
	if err != nil {
		t.Fatalf("second Load returned error: %v", err)
	}

	if first == second {
		t.Fatal("expected different config values from different environment variables")
	}
	if second.Host != "http://host-two:11434" {
		t.Errorf("expected second Host %q, got %q", "http://host-two:11434", second.Host)
	}
	if second.Model != "llama3-two" {
		t.Errorf("expected second Model %q, got %q", "llama3-two", second.Model)
	}
	if second.Timeout != 5*time.Second {
		t.Errorf("expected second Timeout %s, got %s", 5*time.Second, second.Timeout)
	}
}
