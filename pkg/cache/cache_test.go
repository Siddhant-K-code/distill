package cache

import (
	"context"
	"testing"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

func TestMemoryCache_GetSet(t *testing.T) {
	cache := NewMemoryCache(Config{
		MaxSize:    100,
		DefaultTTL: time.Hour,
	})
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Test Set and Get
	err := cache.Set(ctx, "key1", []byte("value1"), 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	value, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(value) != "value1" {
		t.Errorf("expected 'value1', got '%s'", string(value))
	}

	// Test miss
	_, err = cache.Get(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryCache_Delete(t *testing.T) {
	cache := NewMemoryCache(DefaultConfig())
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	_ = cache.Set(ctx, "key1", []byte("value1"), 0)

	err := cache.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = cache.Get(ctx, "key1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}

	// Delete nonexistent key
	err = cache.Delete(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for nonexistent key, got %v", err)
	}
}

func TestMemoryCache_Has(t *testing.T) {
	cache := NewMemoryCache(DefaultConfig())
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	if cache.Has(ctx, "key1") {
		t.Error("expected Has to return false for nonexistent key")
	}

	_ = cache.Set(ctx, "key1", []byte("value1"), 0)

	if !cache.Has(ctx, "key1") {
		t.Error("expected Has to return true for existing key")
	}
}

func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache(DefaultConfig())
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	_ = cache.Set(ctx, "key1", []byte("value1"), 0)
	_ = cache.Set(ctx, "key2", []byte("value2"), 0)
	_ = cache.Set(ctx, "key3", []byte("value3"), 0)

	err := cache.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("expected size 0 after clear, got %d", stats.Size)
	}
}

func TestMemoryCache_TTL(t *testing.T) {
	cache := NewMemoryCache(Config{
		MaxSize:         100,
		CleanupInterval: 10 * time.Millisecond,
	})
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Set with short TTL
	_ = cache.Set(ctx, "key1", []byte("value1"), 50*time.Millisecond)

	// Should exist immediately
	if !cache.Has(ctx, "key1") {
		t.Error("expected key to exist immediately after set")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	_, err := cache.Get(ctx, "key1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after TTL, got %v", err)
	}
}

func TestMemoryCache_LRUEviction(t *testing.T) {
	cache := NewMemoryCache(Config{
		MaxSize:    3,
		DefaultTTL: time.Hour,
	})
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	// Fill cache
	_ = cache.Set(ctx, "key1", []byte("value1"), 0)
	_ = cache.Set(ctx, "key2", []byte("value2"), 0)
	_ = cache.Set(ctx, "key3", []byte("value3"), 0)

	// Access key1 to make it recently used
	_, _ = cache.Get(ctx, "key1")

	// Add new key, should evict key2 (least recently used)
	_ = cache.Set(ctx, "key4", []byte("value4"), 0)

	// key2 should be evicted
	if cache.Has(ctx, "key2") {
		t.Error("expected key2 to be evicted")
	}

	// key1 should still exist (was accessed)
	if !cache.Has(ctx, "key1") {
		t.Error("expected key1 to still exist")
	}

	stats := cache.Stats()
	if stats.Evictions == 0 {
		t.Error("expected at least one eviction")
	}
}

func TestMemoryCache_Stats(t *testing.T) {
	cache := NewMemoryCache(DefaultConfig())
	defer func() { _ = cache.Close() }()

	ctx := context.Background()

	_ = cache.Set(ctx, "key1", []byte("value1"), 0)
	_, _ = cache.Get(ctx, "key1")
	_, _ = cache.Get(ctx, "nonexistent")

	stats := cache.Stats()

	if stats.Sets != 1 {
		t.Errorf("expected 1 set, got %d", stats.Sets)
	}
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Size != 1 {
		t.Errorf("expected size 1, got %d", stats.Size)
	}
}

func TestStats_HitRate(t *testing.T) {
	tests := []struct {
		hits   int64
		misses int64
		want   float64
	}{
		{0, 0, 0},
		{10, 0, 100},
		{0, 10, 0},
		{50, 50, 50},
		{75, 25, 75},
	}

	for _, tt := range tests {
		stats := Stats{Hits: tt.hits, Misses: tt.misses}
		got := stats.HitRate()
		if got != tt.want {
			t.Errorf("HitRate(%d, %d) = %f, want %f", tt.hits, tt.misses, got, tt.want)
		}
	}
}

func TestPatternDetector_DetectPattern(t *testing.T) {
	detector := NewPatternDetector()

	tests := []struct {
		name     string
		text     string
		wantType PatternType
	}{
		{
			name:     "system prompt",
			text:     "You are a helpful AI assistant that helps users with coding tasks. Be concise and accurate.",
			wantType: PatternTypeSystem,
		},
		{
			name:     "tool definition",
			text:     `{"type": "function", "function": {"name": "search", "description": "Search the web", "parameters": {}}}`,
			wantType: PatternTypeTool,
		},
		{
			name:     "code block",
			text:     "Here is the implementation:\n```python\ndef hello():\n    print('hello')\n```",
			wantType: PatternTypeCode,
		},
		{
			name:     "document",
			text:     "This is a regular document with some information about the product and its features.",
			wantType: PatternTypeDocument,
		},
		{
			name:     "short text",
			text:     "Too short",
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := detector.DetectPattern(tt.text)
			if tt.wantType == "" {
				if pattern != nil {
					t.Errorf("expected nil pattern for short text")
				}
				return
			}
			if pattern == nil {
				t.Fatalf("expected pattern, got nil")
			}
			if pattern.Type != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, pattern.Type)
			}
			if pattern.Hash == "" {
				t.Error("expected non-empty hash")
			}
		})
	}
}

func TestPatternDetector_DetectChunkPatterns(t *testing.T) {
	detector := NewPatternDetector()

	chunks := []types.Chunk{
		{ID: "1", Text: "You are a helpful assistant that answers questions accurately and concisely."},
		{ID: "2", Text: "Short"},
		{ID: "3", Text: "This is a document about machine learning and artificial intelligence concepts."},
	}

	patterns := detector.DetectChunkPatterns(chunks)

	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(patterns))
	}
}

func TestHashText(t *testing.T) {
	hash1 := HashText("hello world")
	hash2 := HashText("hello world")
	hash3 := HashText("different text")

	if hash1 != hash2 {
		t.Error("same text should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different text should produce different hash")
	}
	if len(hash1) != 16 {
		t.Errorf("expected hash length 16, got %d", len(hash1))
	}
}

func TestCacheKey(t *testing.T) {
	pattern := &DetectedPattern{
		Type: PatternTypeSystem,
		Hash: "abc123",
	}

	key := CacheKey("distill", pattern)
	expected := "distill:system_prompt:abc123"

	if key != expected {
		t.Errorf("expected '%s', got '%s'", expected, key)
	}
}

func TestCacheKeyForQuery(t *testing.T) {
	key1 := CacheKeyForQuery("distill", "how to reset password", 10)
	key2 := CacheKeyForQuery("distill", "how to reset password", 10)
	key3 := CacheKeyForQuery("distill", "how to reset password", 20)
	key4 := CacheKeyForQuery("distill", "different query", 10)

	if key1 != key2 {
		t.Error("same query and topK should produce same key")
	}
	if key1 == key3 {
		t.Error("different topK should produce different key")
	}
	if key1 == key4 {
		t.Error("different query should produce different key")
	}
}

func TestEntry_IsExpired(t *testing.T) {
	// Not expired (zero time)
	entry1 := Entry{ExpiresAt: time.Time{}}
	if entry1.IsExpired() {
		t.Error("entry with zero ExpiresAt should not be expired")
	}

	// Not expired (future)
	entry2 := Entry{ExpiresAt: time.Now().Add(time.Hour)}
	if entry2.IsExpired() {
		t.Error("entry with future ExpiresAt should not be expired")
	}

	// Expired (past)
	entry3 := Entry{ExpiresAt: time.Now().Add(-time.Hour)}
	if !entry3.IsExpired() {
		t.Error("entry with past ExpiresAt should be expired")
	}
}

func BenchmarkMemoryCache_Get(b *testing.B) {
	cache := NewMemoryCache(DefaultConfig())
	defer func() { _ = cache.Close() }()

	ctx := context.Background()
	_ = cache.Set(ctx, "key", []byte("value"), 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cache.Get(ctx, "key")
	}
}

func BenchmarkMemoryCache_Set(b *testing.B) {
	cache := NewMemoryCache(Config{
		MaxSize:    1000000,
		DefaultTTL: time.Hour,
	})
	defer func() { _ = cache.Close() }()

	ctx := context.Background()
	value := []byte("value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Set(ctx, "key", value, 0)
	}
}

func BenchmarkHashText(b *testing.B) {
	text := "This is a sample text that needs to be hashed for caching purposes."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashText(text)
	}
}
