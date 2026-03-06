package webcache

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"
)

// Tiered chains an L1 (fast, volatile) and L2 (persistent) cache.
// Get checks L1 first; on L2 hit the value is promoted to L1.
// Set writes to both tiers.
type Tiered struct {
	l1  Cache
	l2  Cache
	ttl time.Duration // default TTL used for L1 promotion
	sf  singleflight.Group
}

// NewTiered creates a tiered cache. l2 may be nil (L1-only mode).
// defaultTTL is used when promoting L2 hits into L1.
func NewTiered(l1 Cache, l2 Cache, defaultTTL time.Duration) *Tiered {
	return &Tiered{l1: l1, l2: l2, ttl: defaultTTL}
}

// Get checks L1 first, then L2. On L2 hit, promotes to L1.
func (t *Tiered) Get(ctx context.Context, key string) ([]byte, bool) {
	// L1 check.
	if data, ok := t.l1.Get(ctx, key); ok {
		slog.Debug("webcache: L1 hit", slog.String("key", key))
		return data, true
	}

	// L2 check.
	if t.l2 == nil {
		return nil, false
	}
	data, ok := t.l2.Get(ctx, key)
	if !ok {
		return nil, false
	}

	slog.Debug("webcache: L2 hit, promoting to L1", slog.String("key", key))
	// Promote to L1 -- ignore error since L1 is best-effort.
	_ = t.l1.Set(ctx, key, data, t.ttl)
	return data, true
}

// Set writes to both L1 and L2.
func (t *Tiered) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	// Always write L1.
	if err := t.l1.Set(ctx, key, val, ttl); err != nil {
		return err
	}
	// Write L2 if available.
	if t.l2 != nil {
		if err := t.l2.Set(ctx, key, val, ttl); err != nil {
			slog.Debug("webcache: L2 set failed",
				slog.String("key", key), slog.Any("error", err))
		}
	}
	return nil
}

// Stats returns combined hit/miss counters from both tiers.
func (t *Tiered) Stats() (hits, misses int64) {
	h1, m1 := t.l1.Stats()
	if t.l2 != nil {
		h2, m2 := t.l2.Stats()
		return h1 + h2, m1 + m2
	}
	return h1, m1
}

// GetOrFetch returns a cached value or calls fetchFn exactly once per key.
// Concurrent callers for the same key share the single fetch result
// (singleflight deduplication). On success, the result is cached in both tiers.
func (t *Tiered) GetOrFetch(ctx context.Context, key string, ttl time.Duration,
	fetchFn func(ctx context.Context) ([]byte, error)) ([]byte, error) {

	if val, ok := t.Get(ctx, key); ok {
		return val, nil
	}

	v, err, _ := t.sf.Do(key, func() (any, error) {
		val, fetchErr := fetchFn(ctx)
		if fetchErr != nil {
			return nil, fetchErr
		}
		_ = t.Set(ctx, key, val, ttl)
		return val, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}
