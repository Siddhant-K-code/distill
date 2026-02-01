package cache

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryCache is an in-memory LRU cache with TTL support.
type MemoryCache struct {
	mu       sync.RWMutex
	items    map[string]*list.Element
	lru      *list.List
	cfg      Config
	stats    Stats
	stopCh   chan struct{}
	stopped  atomic.Bool
}

type cacheItem struct {
	entry Entry
}

// NewMemoryCache creates a new in-memory LRU cache.
func NewMemoryCache(cfg Config) *MemoryCache {
	if cfg.MaxSize == 0 {
		cfg.MaxSize = DefaultConfig().MaxSize
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = DefaultConfig().CleanupInterval
	}

	c := &MemoryCache{
		items:  make(map[string]*list.Element),
		lru:    list.New(),
		cfg:    cfg,
		stopCh: make(chan struct{}),
		stats: Stats{
			MaxSize:      cfg.MaxSize,
			MaxSizeBytes: cfg.MaxSizeBytes,
		},
	}

	// Start cleanup goroutine
	go c.cleanupLoop()

	return c
}

// Get retrieves a value by key.
func (c *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		atomic.AddInt64(&c.stats.Misses, 1)
		return nil, ErrNotFound
	}

	item := elem.Value.(*cacheItem)

	// Check expiration
	if item.entry.IsExpired() {
		c.removeElement(elem)
		atomic.AddInt64(&c.stats.Misses, 1)
		atomic.AddInt64(&c.stats.Expirations, 1)
		return nil, ErrNotFound
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(elem)
	atomic.AddInt64(&c.stats.Hits, 1)

	return item.entry.Value, nil
}

// Set stores a value with optional TTL.
func (c *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := int64(len(key) + len(value))

	// Check size limits
	if c.cfg.MaxSizeBytes > 0 && size > c.cfg.MaxSizeBytes {
		return ErrValueTooLarge
	}

	// Calculate expiration
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	} else if c.cfg.DefaultTTL > 0 {
		expiresAt = time.Now().Add(c.cfg.DefaultTTL)
	}

	entry := Entry{
		Key:       key,
		Value:     value,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		Size:      size,
	}

	// Update existing entry
	if elem, ok := c.items[key]; ok {
		oldItem := elem.Value.(*cacheItem)
		atomic.AddInt64(&c.stats.SizeBytes, -oldItem.entry.Size)
		elem.Value = &cacheItem{entry: entry}
		c.lru.MoveToFront(elem)
		atomic.AddInt64(&c.stats.SizeBytes, size)
		atomic.AddInt64(&c.stats.Sets, 1)
		return nil
	}

	// Evict if necessary
	for c.needsEviction(size) {
		c.evictOldest()
	}

	// Add new entry
	elem := c.lru.PushFront(&cacheItem{entry: entry})
	c.items[key] = elem
	atomic.AddInt64(&c.stats.Size, 1)
	atomic.AddInt64(&c.stats.SizeBytes, size)
	atomic.AddInt64(&c.stats.Sets, 1)

	return nil
}

// Delete removes a key from the cache.
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return ErrNotFound
	}

	c.removeElement(elem)
	atomic.AddInt64(&c.stats.Deletes, 1)
	return nil
}

// Has checks if a key exists.
func (c *MemoryCache) Has(ctx context.Context, key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, ok := c.items[key]
	if !ok {
		return false
	}

	item := elem.Value.(*cacheItem)
	return !item.entry.IsExpired()
}

// Clear removes all entries.
func (c *MemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.lru.Init()
	atomic.StoreInt64(&c.stats.Size, 0)
	atomic.StoreInt64(&c.stats.SizeBytes, 0)

	return nil
}

// Stats returns cache statistics.
func (c *MemoryCache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Stats{
		Hits:         atomic.LoadInt64(&c.stats.Hits),
		Misses:       atomic.LoadInt64(&c.stats.Misses),
		Sets:         atomic.LoadInt64(&c.stats.Sets),
		Deletes:      atomic.LoadInt64(&c.stats.Deletes),
		Evictions:    atomic.LoadInt64(&c.stats.Evictions),
		Expirations:  atomic.LoadInt64(&c.stats.Expirations),
		Size:         atomic.LoadInt64(&c.stats.Size),
		SizeBytes:    atomic.LoadInt64(&c.stats.SizeBytes),
		MaxSize:      c.cfg.MaxSize,
		MaxSizeBytes: c.cfg.MaxSizeBytes,
	}
}

// Close stops the cleanup goroutine and releases resources.
func (c *MemoryCache) Close() error {
	if c.stopped.CompareAndSwap(false, true) {
		close(c.stopCh)
	}
	return nil
}

// needsEviction checks if we need to evict entries.
func (c *MemoryCache) needsEviction(additionalSize int64) bool {
	if c.cfg.MaxSize > 0 && atomic.LoadInt64(&c.stats.Size) >= c.cfg.MaxSize {
		return true
	}
	if c.cfg.MaxSizeBytes > 0 && atomic.LoadInt64(&c.stats.SizeBytes)+additionalSize > c.cfg.MaxSizeBytes {
		return true
	}
	return false
}

// evictOldest removes the least recently used entry.
func (c *MemoryCache) evictOldest() {
	elem := c.lru.Back()
	if elem == nil {
		return
	}
	c.removeElement(elem)
	atomic.AddInt64(&c.stats.Evictions, 1)
}

// removeElement removes an element from the cache.
func (c *MemoryCache) removeElement(elem *list.Element) {
	item := elem.Value.(*cacheItem)
	delete(c.items, item.entry.Key)
	c.lru.Remove(elem)
	atomic.AddInt64(&c.stats.Size, -1)
	atomic.AddInt64(&c.stats.SizeBytes, -item.entry.Size)
}

// cleanupLoop periodically removes expired entries.
func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(c.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

// cleanup removes expired entries.
func (c *MemoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toRemove []*list.Element

	for elem := c.lru.Back(); elem != nil; elem = elem.Prev() {
		item := elem.Value.(*cacheItem)
		if !item.entry.ExpiresAt.IsZero() && now.After(item.entry.ExpiresAt) {
			toRemove = append(toRemove, elem)
		}
	}

	for _, elem := range toRemove {
		c.removeElement(elem)
		atomic.AddInt64(&c.stats.Expirations, 1)
	}
}
