package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	DefaultLLMProvider = "ollama"
	DefaultLLMHost     = "http://localhost:11434"
	DefaultLLMTimeout  = 30 * time.Second
	DefaultLogLevel    = "info"
)

const (
	envLLMProvider = "LLM_PROVIDER"
	envLLMModel    = "LLM_MODEL"
	envLLMHost     = "LLM_HOST"
	envLLMAPIKey   = "LLM_API_KEY"
	envLLMTimeout  = "LLM_TIMEOUT"
	envTavilyKey   = "TAVILY_API_KEY"
	envLogLevel    = "LOG_LEVEL"
)

// Config 는 Runtime 실행에 필요한 Phase 0 설정값이다.
type Config struct {
	LLMProvider  string
	LLMModel     string
	LLMHost      string
	LLMAPIKey    string
	LLMTimeout   time.Duration
	TavilyAPIKey string
	LogLevel     string
}

// Load 는 현재 작업 디렉터리의 .env와 실제 환경변수를 병합해 설정을 만든다.
func Load() (Config, error) {
	return LoadFile(".env")
}

// LoadFile 은 path의 .env 파일과 실제 환경변수를 병합해 설정을 만든다.
func LoadFile(path string) (Config, error) {
	values, err := readEnvFile(path)
	if err != nil {
		return Config{}, err
	}

	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		values[key] = value
	}

	return build(values)
}

func readEnvFile(path string) (map[string]string, error) {
	values := make(map[string]string)

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return values, nil
		}
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("parse env file %s:%d: expected KEY=VALUE", path, lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("parse env file %s:%d: empty key", path, lineNumber)
		}

		values[key] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan env file: %w", err)
	}

	return values, nil
}

func build(values map[string]string) (Config, error) {
	timeout, err := durationValue(values[envLLMTimeout], DefaultLLMTimeout)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", envLLMTimeout, err)
	}

	return Config{
		LLMProvider:  stringValue(values[envLLMProvider], DefaultLLMProvider),
		LLMModel:     values[envLLMModel],
		LLMHost:      stringValue(values[envLLMHost], DefaultLLMHost),
		LLMAPIKey:    values[envLLMAPIKey],
		LLMTimeout:   timeout,
		TavilyAPIKey: values[envTavilyKey],
		LogLevel:     stringValue(values[envLogLevel], DefaultLogLevel),
	}, nil
}

func stringValue(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func durationValue(value string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	return duration, nil
}
