package llm

import (
	"errors"
	"strings"
)

type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderOllama Provider = "ollama"
)

type ProviderConfig struct {
	Provider string
	Model    string
	Host     string
	APIKey   string
}

type ProviderRequirements struct {
	Model  bool
	Host   bool
	APIKey bool
}

type ClientFactory func(ProviderConfig) (LLMClient, error)

type Registry struct {
	entries map[Provider]registryEntry
}

type registryEntry struct {
	requirements ProviderRequirements
	factory      ClientFactory
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[Provider]registryEntry)}
}

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
