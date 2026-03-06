package webcache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedis_GetSet(t *testing.T) {
	mr := miniredis.RunT(t)

	r := NewRedis("redis://" + mr.Addr())
	if r == nil {
		t.Fatal("NewRedis returned nil")
	}

	ctx := context.Background()

	// Miss.
	_, ok := r.Get(ctx, "missing")
	if ok {
		t.Fatal("expected miss for unknown key")
	}

	// Set and hit.
	if err := r.Set(ctx, "k1", []byte("val"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	data, ok := r.Get(ctx, "k1")
	if !ok {
		t.Fatal("expected hit for k1")
	}
	if string(data) != "val" {
		t.Errorf("Get(k1) = %q, want %q", data, "val")
	}

	// Stats.
	hits, misses := r.Stats()
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
}

func TestRedis_TTL(t *testing.T) {
	mr := miniredis.RunT(t)

	r := NewRedis("redis://" + mr.Addr())
	if r == nil {
		t.Fatal("NewRedis returned nil")
	}

	ctx := context.Background()

	if err := r.Set(ctx, "k1", []byte("v"), 100*time.Millisecond); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Fast-forward miniredis time.
	mr.FastForward(200 * time.Millisecond)

	_, ok := r.Get(ctx, "k1")
	if ok {
		t.Error("expected miss for expired key")
	}
}

func TestRedis_InvalidURL(t *testing.T) {
	r := NewRedis("not-a-url")
	if r != nil {
		t.Error("expected nil for invalid URL")
	}
}

func TestRedis_Unreachable(t *testing.T) {
	r := NewRedis("redis://127.0.0.1:1")
	if r != nil {
		t.Error("expected nil for unreachable Redis")
	}
}
