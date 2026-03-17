package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	OpenAIAPIKey string
	RedisURL     string
	PostgresURL  string
}

// Load 는 .env 파일을 읽어 환경변수를 설정하고 Config를 반환한다.
// 필수 환경변수가 없으면 즉시 에러를 반환한다.
func Load() (*Config, error) {
	_ = godotenv.Load() // .env 파일이 없으면 무시 (환경변수가 이미 설정된 경우 대비)

	cfg := &Config{
		OpenAIAPIKey: os.Getenv("OPENAI_API_KEY"),
		RedisURL:     os.Getenv("REDIS_URL"),
		PostgresURL:  os.Getenv("POSTGRES_URL"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	required := map[string]string{
		"OPENAI_API_KEY": c.OpenAIAPIKey,
		"REDIS_URL":      c.RedisURL,
		"POSTGRES_URL":   c.PostgresURL,
	}

	for key, val := range required {
		if val == "" {
			return fmt.Errorf("config: 필수 환경변수 %s 가 설정되지 않았습니다", key)
		}
	}

	return nil
}
