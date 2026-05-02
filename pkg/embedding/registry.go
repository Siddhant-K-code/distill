package embedding

import (
	"fmt"
	"strings"
)

// ProviderType identifies a supported embedding backend.
type ProviderType string

const (
	ProviderOpenAI ProviderType = "openai"
	ProviderOllama ProviderType = "ollama"
	ProviderCohere ProviderType = "cohere"
)

// ProviderConfig holds the configuration needed to construct any supported
// embedding provider. Only the fields relevant to the chosen Type are used.
type ProviderConfig struct {
	// Type selects the backend. Required.
	Type ProviderType `yaml:"type" json:"type"`

	// APIKey for cloud providers (OpenAI, Cohere). Can also be set via
	// OPENAI_API_KEY / COHERE_API_KEY environment variables.
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`

	// Model overrides the default model for the chosen provider.
	Model string `yaml:"model,omitempty" json:"model,omitempty"`

	// BaseURL overrides the API endpoint (useful for Azure OpenAI or local
	// Ollama instances on non-default ports).
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty"`

	// CacheSize is the number of embeddings to cache in memory.
	// 0 disables the in-memory cache. Default: 10000.
	CacheSize int `yaml:"cache_size,omitempty" json:"cache_size,omitempty"`
}

// ProviderFactory is a function that constructs a Provider from a ProviderConfig.
// Register custom providers with RegisterFactory.
type ProviderFactory func(cfg ProviderConfig) (Provider, error)

var (
	factories = map[ProviderType]ProviderFactory{}
)

// RegisterFactory registers a custom provider factory for the given type.
// Call this from an init() function in your provider package.
func RegisterFactory(t ProviderType, f ProviderFactory) {
	factories[t] = f
}

// NewProvider constructs a Provider from cfg. Built-in providers (openai,
// ollama, cohere) are always available. Custom providers must be registered
// via RegisterFactory before calling NewProvider.
//
// When cfg.CacheSize > 0 (or unset, defaulting to 10000), the returned
// provider is wrapped in a CachedProvider.
func NewProvider(cfg ProviderConfig) (Provider, error) {
	if cfg.Type == "" {
		return nil, fmt.Errorf("embedding provider type is required")
	}

	// Check custom registry first so callers can override built-ins.
	if f, ok := factories[cfg.Type]; ok {
		p, err := f(cfg)
		if err != nil {
			return nil, err
		}
		return maybeCache(p, cfg.CacheSize), nil
	}

	var p Provider
	var err error

	switch strings.ToLower(string(cfg.Type)) {
	case string(ProviderOpenAI):
		p, err = newOpenAI(cfg)
	case string(ProviderOllama):
		p, err = newOllama(cfg)
	case string(ProviderCohere):
		p, err = newCohere(cfg)
	default:
		return nil, fmt.Errorf("unknown embedding provider %q; supported: openai, ollama, cohere", cfg.Type)
	}
	if err != nil {
		return nil, err
	}
	return maybeCache(p, cfg.CacheSize), nil
}

// SupportedProviders returns the list of built-in provider type strings.
func SupportedProviders() []string {
	return []string{
		string(ProviderOpenAI),
		string(ProviderOllama),
		string(ProviderCohere),
	}
}

func maybeCache(p Provider, cacheSize int) Provider {
	if cacheSize < 0 {
		return p // explicitly disabled
	}
	if cacheSize == 0 {
		cacheSize = 10000
	}
	return NewCachedProvider(p, cacheSize)
}

// newOpenAI constructs an OpenAI provider. Imported lazily to avoid a hard
// dependency when only Ollama or Cohere is used.
func newOpenAI(cfg ProviderConfig) (Provider, error) {
	// Import is handled by the openai sub-package; we call it via the
	// registered factory if available, otherwise return a helpful error.
	if f, ok := factories[ProviderOpenAI]; ok {
		return f(cfg)
	}
	return nil, fmt.Errorf("openai provider not registered; import _ \"github.com/Siddhant-K-code/distill/pkg/embedding/openai\"")
}

func newOllama(cfg ProviderConfig) (Provider, error) {
	if f, ok := factories[ProviderOllama]; ok {
		return f(cfg)
	}
	return nil, fmt.Errorf("ollama provider not registered; import _ \"github.com/Siddhant-K-code/distill/pkg/embedding/ollama\"")
}

func newCohere(cfg ProviderConfig) (Provider, error) {
	if f, ok := factories[ProviderCohere]; ok {
		return f(cfg)
	}
	return nil, fmt.Errorf("cohere provider not registered; import _ \"github.com/Siddhant-K-code/distill/pkg/embedding/cohere\"")
}
