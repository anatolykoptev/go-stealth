package webcache

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestKey(t *testing.T) {
	k1 := Key("gs", "search", "hello")
	k2 := Key("gs", "search", "hello")
	k3 := Key("gs", "search", "world")

	if k1 != k2 {
		t.Errorf("same parts produced different keys: %q vs %q", k1, k2)
	}
	if k1 == k3 {
		t.Error("different parts produced same key")
	}
	if len(k1) < 10 {
		t.Errorf("key too short: %q", k1)
	}
}

func TestMemory_GetSet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewMemory(ctx)

	// Miss.
	_, ok := m.Get(ctx, "missing")
	if ok {
		t.Fatal("expected miss for unknown key")
	}

	// Set and hit.
	if err := m.Set(ctx, "k1", []byte("value1"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	data, ok := m.Get(ctx, "k1")
	if !ok {
		t.Fatal("expected hit for k1")
	}
	if string(data) != "value1" {
		t.Errorf("Get(k1) = %q, want %q", data, "value1")
	}

	// Stats.
	hits, misses := m.Stats()
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
}

func TestMemory_Expiry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewMemory(ctx)

	if err := m.Set(ctx, "k1", []byte("v"), 1*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	_, ok := m.Get(ctx, "k1")
	if ok {
		t.Error("expected miss for expired key")
	}
}

func TestMemory_Eviction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewMemory(ctx, WithMaxEntries(3))

	for i := range 5 {
		key := string(rune('a' + i))
		if err := m.Set(ctx, key, []byte(key), time.Minute); err != nil {
			t.Fatalf("Set(%s): %v", key, err)
		}
	}

	// Should have at most maxEntries.
	count := 0
	m.store.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count > 3 {
		t.Errorf("after eviction, count = %d, want <= 3", count)
	}
}

func TestMemory_CopyOnGetSet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewMemory(ctx)

	original := []byte("hello")
	if err := m.Set(ctx, "k", original, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Mutate original -- should not affect cached value.
	original[0] = 'X'

	data, ok := m.Get(ctx, "k")
	if !ok {
		t.Fatal("expected hit")
	}
	if string(data) != "hello" {
		t.Errorf("cached value mutated: got %q, want %q", data, "hello")
	}

	// Mutate returned value -- should not affect cached value.
	data[0] = 'Y'

	data2, _ := m.Get(ctx, "k")
	if string(data2) != "hello" {
		t.Errorf("cached value mutated via Get: got %q, want %q", data2, "hello")
	}
}

func TestMemory_Concurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewMemory(ctx, WithMaxEntries(100))

	var wg sync.WaitGroup
	const goroutines = 50
	const iterations = 100

	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			key := string(rune('A' + (i % 26)))
			for range iterations {
				_ = m.Set(ctx, key, []byte("data"), time.Minute)
				m.Get(ctx, key)
			}
		}()
	}
	wg.Wait()

	hits, misses := m.Stats()
	if hits+misses == 0 {
		t.Error("expected some cache operations")
	}
}
