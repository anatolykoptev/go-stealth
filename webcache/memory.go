package webcache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const defaultCleanupInterval = 5 * time.Minute

// entry stores a cached value with its expiration time.
type entry struct {
	data      []byte
	expiresAt time.Time
}

// Memory is an in-memory cache backed by sync.Map with TTL and eviction.
type Memory struct {
	store           sync.Map
	maxEntries      int
	cleanupInterval time.Duration
	hits            atomic.Int64
	misses          atomic.Int64
}

// MemoryOption configures a Memory cache.
type MemoryOption func(*Memory)

// WithMaxEntries sets the maximum number of entries before eviction triggers.
// Zero or negative disables eviction.
func WithMaxEntries(n int) MemoryOption {
	return func(m *Memory) { m.maxEntries = n }
}

// WithCleanupInterval sets how often expired entries are removed.
// Defaults to 5 minutes.
func WithCleanupInterval(d time.Duration) MemoryOption {
	return func(m *Memory) { m.cleanupInterval = d }
}

// NewMemory creates an in-memory cache.
// The cleanup goroutine runs until ctx is cancelled.
func NewMemory(ctx context.Context, opts ...MemoryOption) *Memory {
	m := &Memory{cleanupInterval: defaultCleanupInterval}
	for _, o := range opts {
		o(m)
	}
	go m.cleanupLoop(ctx)
	return m
}

// Get retrieves a value by key. Returns nil, false on miss or expiry.
func (m *Memory) Get(_ context.Context, key string) ([]byte, bool) {
	val, ok := m.store.Load(key)
	if !ok {
		m.misses.Add(1)
		return nil, false
	}
	e := val.(*entry)
	if time.Now().After(e.expiresAt) {
		m.store.Delete(key)
		m.misses.Add(1)
		return nil, false
	}
	m.hits.Add(1)
	// Return a copy to prevent caller mutations.
	cp := make([]byte, len(e.data))
	copy(cp, e.data)
	return cp, true
}

// Set stores a value with the given TTL.
func (m *Memory) Set(_ context.Context, key string, val []byte, ttl time.Duration) error {
	m.evictIfNeeded()
	// Store a copy to prevent caller mutations.
	cp := make([]byte, len(val))
	copy(cp, val)
	m.store.Store(key, &entry{
		data:      cp,
		expiresAt: time.Now().Add(ttl),
	})
	return nil
}

// Stats returns cache hit and miss counters.
func (m *Memory) Stats() (hits, misses int64) {
	return m.hits.Load(), m.misses.Load()
}

// evictIfNeeded removes entries when the cache exceeds maxEntries.
// Phase 1: remove expired. Phase 2: remove oldest (earliest expiry).
func (m *Memory) evictIfNeeded() {
	if m.maxEntries <= 0 {
		return
	}

	count := 0
	m.store.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count < m.maxEntries {
		return
	}

	// Phase 1: remove expired entries.
	now := time.Now()
	m.store.Range(func(key, val any) bool {
		if e, ok := val.(*entry); ok && now.After(e.expiresAt) {
			m.store.Delete(key)
			count--
		}
		return count >= m.maxEntries
	})
	if count < m.maxEntries {
		return
	}

	// Phase 2: remove oldest entries until under limit.
	m.evictOldest(now, &count)
}

// evictOldest removes the entry with the earliest expiry, repeating until
// the count drops below maxEntries.
func (m *Memory) evictOldest(now time.Time, count *int) {
	var oldest struct {
		key any
		at  time.Time
	}
	for *count >= m.maxEntries {
		oldest.key = nil
		oldest.at = now.Add(time.Hour)
		m.store.Range(func(key, val any) bool {
			if e, ok := val.(*entry); ok && e.expiresAt.Before(oldest.at) {
				oldest.key = key
				oldest.at = e.expiresAt
			}
			return true
		})
		if oldest.key == nil {
			break
		}
		m.store.Delete(oldest.key)
		*count--
	}
}

// cleanupLoop periodically removes expired entries until ctx is cancelled.
func (m *Memory) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			m.store.Range(func(key, val any) bool {
				if e, ok := val.(*entry); ok && now.After(e.expiresAt) {
					m.store.Delete(key)
				}
				return true
			})
		case <-ctx.Done():
			return
		}
	}
}
