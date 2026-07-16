package llm

import (
	"errors"
	"strings"
)

// Provider 타입은 Runtime이 선택할 수 있는 LLM 공급자 식별자다.
type Provider string

const (
	// ProviderClaude 상수는 Anthropic Claude Messages API 공급자를 식별한다.
	ProviderClaude Provider = "claude"
	// ProviderOllama 상수는 로컬 Ollama Chat API 공급자를 식별한다.
	ProviderOllama Provider = "ollama"
)

// ProviderConfig 구조체는 공급자 선택과 클라이언트 생성에 필요한 공통 설정이다.
type ProviderConfig struct {
	Provider string
	Model    string
	Host     string
	APIKey   string
}

// ProviderRequirements 구조체는 공급자별로 비어 있지 않아야 하는 설정 필드를 표시한다.
type ProviderRequirements struct {
	Model  bool
	Host   bool
	APIKey bool
}

// ClientFactory 함수 타입은 검증된 공급자 설정으로 LLM 클라이언트를 생성한다.
type ClientFactory func(ProviderConfig) (LLMClient, error)

// Registry 구조체는 공급자별 요구 조건과 클라이언트 생성 함수를 보관한다.
type Registry struct {
	entries map[Provider]registryEntry
}

type registryEntry struct {
	requirements ProviderRequirements
	factory      ClientFactory
}

// NewRegistry 함수는 비어 있는 공급자 레지스트리를 만든다.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[Provider]registryEntry)}
}

// Register 메서드는 공급자 이름을 정규화해 생성 함수를 등록한다.
// 같은 공급자를 다시 등록하면 기존 항목을 새 항목으로 교체한다.
func (r *Registry) Register(provider Provider, requirements ProviderRequirements, factory ClientFactory) error {
	if r == nil {
		return configError(provider, "register provider", errors.New("nil registry"))
	}
	provider = normalizeProvider(string(provider))
	if provider == "" {
		return configError("", "register provider", errors.New("provider is required"))
	}
	if factory == nil {
		return configError(provider, "register provider", errors.New("factory is required"))
	}

	r.entries[provider] = registryEntry{
		requirements: requirements,
		factory:      factory,
	}
	return nil
}

// NewClient 메서드는 공급자 필수 설정을 검증한 뒤 등록된 생성 함수로 클라이언트를 만든다.
func (r *Registry) NewClient(cfg ProviderConfig) (LLMClient, error) {
	if r == nil {
		return nil, configError("", "select provider", errors.New("nil registry"))
	}

	provider := normalizeProvider(cfg.Provider)
	entry, ok := r.entries[provider]
	if !ok {
		return nil, configError(provider, "select provider", errors.New("unsupported provider"))
	}

	normalized := cfg
	normalized.Provider = string(provider)
	if err := validateRequired(normalized, entry.requirements); err != nil {
		return nil, err
	}

	return entry.factory(normalized)
}

func normalizeProvider(provider string) Provider {
	return Provider(strings.ToLower(strings.TrimSpace(provider)))
}

func validateRequired(cfg ProviderConfig, requirements ProviderRequirements) error {
	provider := normalizeProvider(cfg.Provider)
	if requirements.Model && strings.TrimSpace(cfg.Model) == "" {
		return configError(provider, "validate provider config", errors.New("model is required"))
	}
	if requirements.Host && strings.TrimSpace(cfg.Host) == "" {
		return configError(provider, "validate provider config", errors.New("host is required"))
	}
	if requirements.APIKey && strings.TrimSpace(cfg.APIKey) == "" {
		return configError(provider, "validate provider config", errors.New("api key is required"))
	}
	return nil
}

func configError(provider Provider, op string, err error) error {
	return &Error{
		Kind:     ErrorKindConfig,
		Provider: provider,
		Op:       op,
		Err:      err,
	}
}
