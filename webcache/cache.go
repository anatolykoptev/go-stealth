// Package webcache provides a two-tier caching layer (L1 memory + L2 Redis)
// with automatic promotion on L2 hits and singleflight deduplication.
//
// The [Cache] interface is implemented by [Memory], [Redis], and [Tiered].
// [Tiered] chains L1 and L2 with automatic promotion on L2 hits.
package webcache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// Cache is the interface for key-value storage with TTL.
type Cache interface {
	// Get retrieves a value by key. Returns nil, false on miss.
	Get(ctx context.Context, key string) ([]byte, bool)

	// Set stores a value with the given TTL.
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error

	// Stats returns cache hit and miss counters.
	Stats() (hits, misses int64)
}

// Key builds a deterministic cache key from parts using SHA256.
// Returns a 24-char hex string with the given prefix.
func Key(prefix string, parts ...string) string {
	joined := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(joined))
	return fmt.Sprintf("%s:%x", prefix, hash[:12])
}
