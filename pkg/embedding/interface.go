package embedding

import (
	"context"
	"errors"
)

// Common errors returned by embedding providers.
var (
	ErrEmptyInput     = errors.New("empty input text")
	ErrRateLimited    = errors.New("rate limited by embedding provider")
	ErrInvalidAPIKey  = errors.New("invalid API key")
	ErrModelNotFound  = errors.New("embedding model not found")
	ErrContextTooLong = errors.New("input text exceeds model context length")
)

// Provider defines the interface for text embedding services.
type Provider interface {
	// Embed converts a single text into a vector embedding.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch converts multiple texts into vector embeddings.
	// More efficient than calling Embed multiple times.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimension returns the embedding dimension for this provider.
	Dimension() int

	// ModelName returns the name of the embedding model.
	ModelName() string
}

// CachedProvider wraps a Provider with an in-memory cache.
type CachedProvider struct {
	provider Provider
	cache    map[string][]float32
	maxSize  int
}

// NewCachedProvider creates a cached embedding provider.
func NewCachedProvider(provider Provider, maxSize int) *CachedProvider {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &CachedProvider{
		provider: provider,
		cache:    make(map[string][]float32),
		maxSize:  maxSize,
	}
}

// Embed returns cached embedding or computes and caches it.
func (c *CachedProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if cached, ok := c.cache[text]; ok {
		// Return a copy to prevent mutation
		result := make([]float32, len(cached))
		copy(result, cached)
		return result, nil
	}

	embedding, err := c.provider.Embed(ctx, text)
	if err != nil {
		return nil, err
	}

	// Cache if under limit
	if len(c.cache) < c.maxSize {
		cached := make([]float32, len(embedding))
		copy(cached, embedding)
		c.cache[text] = cached
	}

	return embedding, nil
}

// EmbedBatch embeds multiple texts, using cache where available.
func (c *CachedProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	uncached := make([]string, 0)
	uncachedIdx := make([]int, 0)

	// Check cache
	for i, text := range texts {
		if cached, ok := c.cache[text]; ok {
			result := make([]float32, len(cached))
			copy(result, cached)
			results[i] = result
		} else {
			uncached = append(uncached, text)
			uncachedIdx = append(uncachedIdx, i)
		}
	}

	// Embed uncached texts
	if len(uncached) > 0 {
		embeddings, err := c.provider.EmbedBatch(ctx, uncached)
		if err != nil {
			return nil, err
		}

		for i, embedding := range embeddings {
			idx := uncachedIdx[i]
			results[idx] = embedding

			// Cache if under limit
			if len(c.cache) < c.maxSize {
				cached := make([]float32, len(embedding))
				copy(cached, embedding)
				c.cache[uncached[i]] = cached
			}
		}
	}

	return results, nil
}

// Dimension returns the embedding dimension.
func (c *CachedProvider) Dimension() int {
	return c.provider.Dimension()
}

// ModelName returns the model name.
func (c *CachedProvider) ModelName() string {
	return c.provider.ModelName()
}

// CacheSize returns the current cache size.
func (c *CachedProvider) CacheSize() int {
	return len(c.cache)
}

// ClearCache clears the embedding cache.
func (c *CachedProvider) ClearCache() {
	c.cache = make(map[string][]float32)
}
