package webcache

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisPingTimeout = 3 * time.Second

// Redis is a cache backed by Redis.
type Redis struct {
	rdb    *redis.Client
	hits   atomic.Int64
	misses atomic.Int64
}

// NewRedis creates a Redis cache from a connection URL (e.g. "redis://localhost:6379/0").
// Returns nil if the URL is invalid or Redis is unreachable.
func NewRedis(redisURL string) *Redis {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Warn("webcache: invalid redis URL", slog.Any("error", err))
		return nil
	}
	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), redisPingTimeout)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("webcache: redis unreachable", slog.Any("error", err))
		return nil
	}

	slog.Info("webcache: redis connected", slog.String("addr", opts.Addr))
	return &Redis{rdb: rdb}
}

// Get retrieves a value from Redis. Returns nil, false on miss.
func (r *Redis) Get(ctx context.Context, key string) ([]byte, bool) {
	data, err := r.rdb.Get(ctx, key).Bytes()
	if err != nil {
		r.misses.Add(1)
		return nil, false
	}
	r.hits.Add(1)
	return data, true
}

// Set stores a value in Redis with the given TTL.
func (r *Redis) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	if err := r.rdb.Set(ctx, key, val, ttl).Err(); err != nil {
		slog.Debug("webcache: redis set failed",
			slog.String("key", key), slog.Any("error", err))
		return err
	}
	return nil
}

// Stats returns cache hit and miss counters.
func (r *Redis) Stats() (hits, misses int64) {
	return r.hits.Load(), r.misses.Load()
}
