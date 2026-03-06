package websearch

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type providerCounter struct {
	calls atomic.Int32
}

func (c *providerCounter) Search(_ context.Context, _ string, _ SearchOpts) ([]Result, error) {
	c.calls.Add(1)
	return []Result{{Title: "ok", URL: "http://ok.com"}}, nil
}

func TestRateLimited_Throttles(t *testing.T) {
	t.Parallel()
	counter := &providerCounter{}
	limited := NewRateLimited(counter, 2, 1) // 2 req/s, burst 1

	ctx := context.Background()

	// First call — immediate (burst token).
	_, err := limited.Search(ctx, "q1", SearchOpts{})
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	// Rapid second call — should block briefly.
	start := time.Now()
	_, err = limited.Search(ctx, "q2", SearchOpts{})
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	elapsed := time.Since(start)

	// Should have waited ~500ms (1/2 req/s).
	if elapsed < 300*time.Millisecond {
		t.Errorf("expected throttle delay, got %v", elapsed)
	}

	if counter.calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", counter.calls.Load())
	}
}

func TestRateLimited_ContextCancel(t *testing.T) {
	t.Parallel()
	counter := &providerCounter{}
	limited := NewRateLimited(counter, 0.1, 1) // very slow: 1 per 10s

	ctx := context.Background()
	// Consume burst token.
	_, _ = limited.Search(ctx, "q1", SearchOpts{})

	// Cancel context — should error, not block.
	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := limited.Search(ctx2, "q2", SearchOpts{})
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestRateLimited_PassesThrough(t *testing.T) {
	t.Parallel()
	inner := &providerCounter{}
	limited := NewRateLimited(inner, 100, 10) // generous limit

	results, err := limited.Search(context.Background(), "test", SearchOpts{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "ok" {
		t.Errorf("unexpected results: %v", results)
	}
}
