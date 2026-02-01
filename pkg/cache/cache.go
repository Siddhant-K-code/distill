// Package cache provides KV caching for repeated context patterns.
// It enables sub-millisecond retrieval for common prompts, tool definitions,
// and other frequently accessed content.
package cache

import (
	"context"
	"errors"
	"time"
)

// Common errors.
var (
	ErrNotFound     = errors.New("key not found")
	ErrKeyTooLarge  = errors.New("key exceeds maximum size")
	ErrValueTooLarge = errors.New("value exceeds maximum size")
	ErrCacheFull    = errors.New("cache is full")
)

// Cache defines the interface for KV caching.
type Cache interface {
	// Get retrieves a value by key. Returns ErrNotFound if not present.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value with optional TTL. Zero TTL means no expiration.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a key from the cache.
	Delete(ctx context.Context, key string) error

	// Has checks if a key exists without retrieving the value.
	Has(ctx context.Context, key string) bool

	// Clear removes all entries from the cache.
	Clear(ctx context.Context) error

	// Stats returns cache statistics.
	Stats() Stats

	// Close releases resources.
	Close() error
}

// Stats holds cache performance metrics.
type Stats struct {
	// Hits is the number of successful cache retrievals.
	Hits int64

	// Misses is the number of cache misses.
	Misses int64

	// Sets is the number of cache writes.
	Sets int64

	// Deletes is the number of cache deletions.
	Deletes int64

	// Evictions is the number of entries evicted due to size limits.
	Evictions int64

	// Expirations is the number of entries expired due to TTL.
	Expirations int64

	// Size is the current number of entries.
	Size int64

	// SizeBytes is the current memory usage in bytes.
	SizeBytes int64

	// MaxSize is the maximum number of entries allowed.
	MaxSize int64

	// MaxSizeBytes is the maximum memory allowed in bytes.
	MaxSizeBytes int64
}

// HitRate returns the cache hit rate as a percentage.
func (s Stats) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total) * 100
}

// Config holds cache configuration.
type Config struct {
	// MaxSize is the maximum number of entries (0 = unlimited).
	MaxSize int64

	// MaxSizeBytes is the maximum memory in bytes (0 = unlimited).
	MaxSizeBytes int64

	// DefaultTTL is the default expiration time for entries without explicit TTL.
	DefaultTTL time.Duration

	// CleanupInterval is how often to run expiration cleanup.
	CleanupInterval time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxSize:         10000,
		MaxSizeBytes:    100 * 1024 * 1024, // 100MB
		DefaultTTL:      time.Hour,
		CleanupInterval: time.Minute,
	}
}

// Entry represents a cached item.
type Entry struct {
	Key       string
	Value     []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	Size      int64
}

// IsExpired checks if the entry has expired.
func (e Entry) IsExpired() bool {
	if e.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(e.ExpiresAt)
}
