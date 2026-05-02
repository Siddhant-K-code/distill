package embedding_test

import (
	"context"
	"testing"

	"github.com/Siddhant-K-code/distill/pkg/embedding"
	_ "github.com/Siddhant-K-code/distill/pkg/embedding/ollama"
)

// mockProvider is a minimal Provider for testing the registry.
type mockProvider struct{ dim int }

func (m *mockProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, m.dim), nil
}
func (m *mockProvider) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, m.dim)
	}
	return out, nil
}
func (m *mockProvider) Dimension() int    { return m.dim }
func (m *mockProvider) ModelName() string { return "mock" }

func TestRegisterFactory_CustomProvider(t *testing.T) {
	embedding.RegisterFactory("mock", func(cfg embedding.ProviderConfig) (embedding.Provider, error) {
		return &mockProvider{dim: 128}, nil
	})

	p, err := embedding.NewProvider(embedding.ProviderConfig{
		Type:      "mock",
		CacheSize: -1, // disable cache for test
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Dimension() != 128 {
		t.Errorf("expected dim 128, got %d", p.Dimension())
	}
}

func TestNewProvider_UnknownType(t *testing.T) {
	_, err := embedding.NewProvider(embedding.ProviderConfig{Type: "nonexistent"})
	if err == nil {
		t.Error("expected error for unknown provider type")
	}
}

func TestNewProvider_EmptyType(t *testing.T) {
	_, err := embedding.NewProvider(embedding.ProviderConfig{})
	if err == nil {
		t.Error("expected error for empty provider type")
	}
}

func TestNewProvider_OllamaRegistered(t *testing.T) {
	// Ollama is registered via the blank import above.
	// We can't make a real HTTP call, but we can verify the factory resolves.
	p, err := embedding.NewProvider(embedding.ProviderConfig{
		Type:      embedding.ProviderOllama,
		CacheSize: -1,
	})
	if err != nil {
		t.Fatalf("expected ollama provider to resolve, got: %v", err)
	}
	if p.ModelName() == "" {
		t.Error("expected non-empty model name")
	}
}

func TestSupportedProviders(t *testing.T) {
	providers := embedding.SupportedProviders()
	if len(providers) != 3 {
		t.Errorf("expected 3 supported providers, got %d", len(providers))
	}
	want := map[string]bool{"openai": true, "ollama": true, "cohere": true}
	for _, p := range providers {
		if !want[p] {
			t.Errorf("unexpected provider %q", p)
		}
	}
}

func TestCachedProvider_WrapsWhenCacheSizePositive(t *testing.T) {
	embedding.RegisterFactory("mock2", func(cfg embedding.ProviderConfig) (embedding.Provider, error) {
		return &mockProvider{dim: 64}, nil
	})

	p, err := embedding.NewProvider(embedding.ProviderConfig{
		Type:      "mock2",
		CacheSize: 100,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	// CachedProvider wraps the mock — verify it still satisfies the interface.
	if p.Dimension() != 64 {
		t.Errorf("expected dim 64 through cache wrapper, got %d", p.Dimension())
	}
}
