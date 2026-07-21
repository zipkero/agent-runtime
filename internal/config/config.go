// Package config는 Runtime 실행 설정을 .env 파일과 환경변수에서 구성한다.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultLLMProvider 상수는 공급자가 지정되지 않았을 때 선택하는 기본값이다.
	DefaultLLMProvider = "ollama"
	// DefaultLLMHost 상수는 로컬 Ollama의 기본 주소다.
	DefaultLLMHost = "http://localhost:11434"
	// DefaultLLMTimeout 상수는 LLM 호출에 적용하는 기본 제한 시간이다.
	DefaultLLMTimeout = 30 * time.Second
	// DefaultLogLevel 상수는 로그 레벨이 지정되지 않았을 때 사용하는 기본값이다.
	DefaultLogLevel = "info"
)

const (
	envLLMProvider         = "LLM_PROVIDER"
	envLLMModel            = "LLM_MODEL"
	envLLMHost             = "LLM_HOST"
	envLLMAPIKey           = "LLM_API_KEY"
	envLLMTimeout          = "LLM_TIMEOUT"
	envTavilyKey           = "TAVILY_API_KEY"
	envEnableCodeExecution = "ENABLE_CODE_EXECUTION"
	envLogLevel            = "LOG_LEVEL"
)

// Config 구조체는 Runtime 실행과 외부 공급자 연결에 필요한 설정값이다.
type Config struct {
	LLMProvider         string
	LLMModel            string
	LLMHost             string
	LLMAPIKey           string
	LLMTimeout          time.Duration
	TavilyAPIKey        string
	EnableCodeExecution bool
	LogLevel            string
}

// Load 함수는 현재 작업 디렉터리의 .env와 실제 환경변수를 병합해 설정을 만든다.
func Load() (Config, error) {
	return LoadFile(".env")
}

// LoadFile 함수는 지정한 .env 파일과 실제 환경변수를 병합해 설정을 만든다.
// 파일이 없으면 빈 설정으로 계속하며, 같은 키는 실제 환경변수가 파일 값을 덮어쓴다.
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
	if timeout <= 0 {
		return Config{}, fmt.Errorf("%s: duration must be positive", envLLMTimeout)
	}
	enableCodeExecution, err := boolValue(values[envEnableCodeExecution], false)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", envEnableCodeExecution, err)
	}

	return Config{
		LLMProvider:         stringValue(values[envLLMProvider], DefaultLLMProvider),
		LLMModel:            values[envLLMModel],
		LLMHost:             stringValue(values[envLLMHost], DefaultLLMHost),
		LLMAPIKey:           values[envLLMAPIKey],
		LLMTimeout:          timeout,
		TavilyAPIKey:        values[envTavilyKey],
		EnableCodeExecution: enableCodeExecution,
		LogLevel:            stringValue(values[envLogLevel], DefaultLogLevel),
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

func boolValue(value string, fallback bool) (bool, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback, nil
	}
	return strconv.ParseBool(trimmed)
}
