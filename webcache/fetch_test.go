package webcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTiered_GetOrFetch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l1 := NewMemory(ctx)
	tiered := NewTiered(l1, nil, time.Minute)

	var fetchCount atomic.Int64
	val, err := tiered.GetOrFetch(ctx, "key1", time.Minute,
		func(_ context.Context) ([]byte, error) {
			fetchCount.Add(1)
			return []byte("fetched-value"), nil
		})
	if err != nil {
		t.Fatalf("GetOrFetch: %v", err)
	}
	if string(val) != "fetched-value" {
		t.Errorf("value = %q, want %q", val, "fetched-value")
	}
	if fetchCount.Load() != 1 {
		t.Errorf("fetch count = %d, want 1", fetchCount.Load())
	}

	// Second call should hit cache.
	val2, err := tiered.GetOrFetch(ctx, "key1", time.Minute,
		func(_ context.Context) ([]byte, error) {
			fetchCount.Add(1)
			return []byte("should-not-run"), nil
		})
	if err != nil {
		t.Fatalf("GetOrFetch (cached): %v", err)
	}
	if string(val2) != "fetched-value" {
		t.Errorf("cached value = %q, want %q", val2, "fetched-value")
	}
	if fetchCount.Load() != 1 {
		t.Errorf("fetch count = %d, want 1 (cached)", fetchCount.Load())
	}
}

func TestTiered_GetOrFetch_Singleflight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l1 := NewMemory(ctx)
	tiered := NewTiered(l1, nil, time.Minute)

	var fetchCount atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := tiered.GetOrFetch(ctx, "shared-key", time.Minute,
				func(_ context.Context) ([]byte, error) {
					fetchCount.Add(1)
					time.Sleep(10 * time.Millisecond)
					return []byte("shared-value"), nil
				})
			if err != nil {
				t.Errorf("GetOrFetch: %v", err)
				return
			}
			if string(val) != "shared-value" {
				t.Errorf("value = %q", val)
			}
		}()
	}
	wg.Wait()

	if fetchCount.Load() > 2 {
		t.Errorf("fetch count = %d, want <= 2 (singleflight)", fetchCount.Load())
	}
}

func TestTiered_GetOrFetch_Error(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l1 := NewMemory(ctx)
	tiered := NewTiered(l1, nil, time.Minute)

	_, err := tiered.GetOrFetch(ctx, "err-key", time.Minute,
		func(_ context.Context) ([]byte, error) {
			return nil, errors.New("fetch failed")
		})
	if err == nil {
		t.Fatal("expected error")
	}

	_, ok := tiered.Get(ctx, "err-key")
	if ok {
		t.Error("failed fetch should not be cached")
	}
}
