// Package config 는 agent-runtime 실행에 필요한 환경변수 기반 설정을 제공한다.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Provider 는 지원 가능한 LLM provider를 나타내는 타입이다.
type Provider string

const (
	ProviderOllama Provider = "ollama"
	ProviderClaude Provider = "claude"
)

const (
	EnvProvider    = "LLM_PROVIDER"
	EnvModel       = "LLM_MODEL"
	EnvHost        = "LLM_HOST"
	EnvAPIKey      = "LLM_API_KEY"
	EnvTimeout     = "LLM_TIMEOUT"
	EnvTavilyAPIKey = "TAVILY_API_KEY"

	DefaultTimeout    = 30 * time.Second
	DefaultOllamaHost = "http://localhost:11434"
)

// Config 는 LLM provider 생성과 호출 timeout 설정에 필요한 값이다.
type Config struct {
	Provider    Provider
	Model       string
	Host        string
	APIKey      string
	Timeout     time.Duration
	TavilyAPIKey string
}

// Load 는 환경변수에서 Config를 읽는다.
// 프로젝트 루트에 .env가 있으면 먼저 로딩하되, 이미 설정된 실제 환경변수를 덮어쓰지 않으므로
// 실제 환경변수가 .env보다 우선한다. .env가 없으면 조용히 건너뛴다.
// LLM_PROVIDER가 미지정이면 ollama로 기본 설정한다.
// 인식 불가 provider 값이거나 선택된 provider에 필요한 값이 부재하면 error를 반환한다.
func Load() (Config, error) {
	_ = godotenv.Load()

	// provider 파싱: 미지정이면 ollama, 미인식 값이면 error
	providerRaw := strings.TrimSpace(os.Getenv(EnvProvider))
	var provider Provider
	if providerRaw == "" {
		provider = ProviderOllama
	} else {
		switch Provider(providerRaw) {
		case ProviderOllama, ProviderClaude:
			provider = Provider(providerRaw)
		default:
			return Config{}, fmt.Errorf("unrecognized %s value %q: must be %q or %q",
				EnvProvider, providerRaw, ProviderOllama, ProviderClaude)
		}
	}

	model := strings.TrimSpace(os.Getenv(EnvModel))
	host := strings.TrimSpace(os.Getenv(EnvHost))
	apiKey := strings.TrimSpace(os.Getenv(EnvAPIKey))

	// 선택된 provider에 필요한 값만 조건부 검증
	switch provider {
	case ProviderOllama:
		if model == "" {
			return Config{}, fmt.Errorf("%s is required for provider %q", EnvModel, provider)
		}
		if host == "" {
			host = DefaultOllamaHost
		}
	case ProviderClaude:
		if apiKey == "" {
			return Config{}, fmt.Errorf("%s is required for provider %q", EnvAPIKey, provider)
		}
		if model == "" {
			return Config{}, fmt.Errorf("%s is required for provider %q", EnvModel, provider)
		}
	}

	timeout, err := loadTimeout()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Provider:     provider,
		Model:        model,
		Host:         host,
		APIKey:       apiKey,
		Timeout:      timeout,
		TavilyAPIKey: strings.TrimSpace(os.Getenv(EnvTavilyAPIKey)),
	}, nil
}

func loadTimeout() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(EnvTimeout))
	if value == "" {
		return DefaultTimeout, nil
	}

	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", EnvTimeout, err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("%s must be greater than 0", EnvTimeout)
	}

	return timeout, nil
}
