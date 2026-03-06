package webcache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestTiered_L1Hit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l1 := NewMemory(ctx)
	mr := miniredis.RunT(t)
	l2 := NewRedis("redis://" + mr.Addr())

	tc := NewTiered(l1, l2, time.Minute)

	_ = l1.Set(ctx, "k", []byte("from-l1"), time.Minute)

	data, ok := tc.Get(ctx, "k")
	if !ok {
		t.Fatal("expected L1 hit")
	}
	if string(data) != "from-l1" {
		t.Errorf("got %q, want %q", data, "from-l1")
	}
}

func TestTiered_L2HitPromotesToL1(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l1 := NewMemory(ctx)
	mr := miniredis.RunT(t)
	l2 := NewRedis("redis://" + mr.Addr())

	tc := NewTiered(l1, l2, time.Minute)

	_ = l2.Set(ctx, "k", []byte("from-l2"), time.Minute)

	data, ok := tc.Get(ctx, "k")
	if !ok {
		t.Fatal("expected L2 hit")
	}
	if string(data) != "from-l2" {
		t.Errorf("got %q, want %q", data, "from-l2")
	}

	// Now should be in L1.
	data2, ok2 := l1.Get(ctx, "k")
	if !ok2 {
		t.Fatal("expected L1 promotion")
	}
	if string(data2) != "from-l2" {
		t.Errorf("L1 promoted value = %q, want %q", data2, "from-l2")
	}
}

func TestTiered_SetWritesBoth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l1 := NewMemory(ctx)
	mr := miniredis.RunT(t)
	l2 := NewRedis("redis://" + mr.Addr())

	tc := NewTiered(l1, l2, time.Minute)

	if err := tc.Set(ctx, "k", []byte("val"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	d1, ok := l1.Get(ctx, "k")
	if !ok || string(d1) != "val" {
		t.Errorf("L1: got %q/%v, want %q/true", d1, ok, "val")
	}

	d2, ok := l2.Get(ctx, "k")
	if !ok || string(d2) != "val" {
		t.Errorf("L2: got %q/%v, want %q/true", d2, ok, "val")
	}
}

func TestTiered_L2Nil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l1 := NewMemory(ctx)
	tc := NewTiered(l1, nil, time.Minute)

	if err := tc.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	data, ok := tc.Get(ctx, "k")
	if !ok || string(data) != "v" {
		t.Errorf("L1-only Get: %q/%v, want %q/true", data, ok, "v")
	}
}

func TestTiered_Stats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l1 := NewMemory(ctx)
	mr := miniredis.RunT(t)
	l2 := NewRedis("redis://" + mr.Addr())

	tc := NewTiered(l1, l2, time.Minute)

	_ = tc.Set(ctx, "k", []byte("v"), time.Minute)
	tc.Get(ctx, "k")    // L1 hit
	tc.Get(ctx, "miss") // total miss

	hits, misses := tc.Stats()
	if hits < 1 {
		t.Errorf("hits = %d, want >= 1", hits)
	}
	if misses < 1 {
		t.Errorf("misses = %d, want >= 1", misses)
	}
}
