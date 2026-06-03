// Package config 는 agent-runtime 실행에 필요한 환경변수 기반 설정을 제공한다.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	EnvAnthropicAPIKey = "ANTHROPIC_API_KEY"
	EnvModel           = "ANTHROPIC_MODEL"
	EnvTimeout         = "LLM_TIMEOUT"

	DefaultTimeout = 30 * time.Second
)

// Config 는 LLM provider 생성과 호출 timeout 설정에 필요한 값이다.
type Config struct {
	AnthropicAPIKey string
	Model           string
	Timeout         time.Duration
}

// Load 는 환경변수에서 Config를 읽는다.
// 프로젝트 루트에 .env가 있으면 먼저 로딩하되, 이미 설정된 실제 환경변수를 덮어쓰지 않으므로
// 실제 환경변수가 .env보다 우선한다. .env가 없으면 조용히 건너뛴다.
func Load() (Config, error) {
	_ = godotenv.Load()

	apiKey := strings.TrimSpace(os.Getenv(EnvAnthropicAPIKey))
	if apiKey == "" {
		return Config{}, fmt.Errorf("%s is required", EnvAnthropicAPIKey)
	}

	model := strings.TrimSpace(os.Getenv(EnvModel))
	if model == "" {
		return Config{}, fmt.Errorf("%s is required", EnvModel)
	}

	timeout, err := loadTimeout()
	if err != nil {
		return Config{}, err
	}

	return Config{
		AnthropicAPIKey: apiKey,
		Model:           model,
		Timeout:         timeout,
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
