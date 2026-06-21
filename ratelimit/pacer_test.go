package ratelimit

import (
	"context"
	"testing"
	"time"
)

// TestKeyedPacer_FirstRequestImmediate verifies the very first request for a
// key is allowed with no delay (no prior timestamp to space against).
func TestKeyedPacer_FirstRequestImmediate(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	p := NewKeyedPacer(time.Second, 0, WithPacerClock(fc.Now))

	if !p.Allow("acctA") {
		t.Fatal("first request for a fresh key must be allowed immediately")
	}
}

// TestKeyedPacer_SpacesPerKey verifies a second request for the SAME key inside
// the MinDelay window is denied, then allowed once the clock advances past it.
func TestKeyedPacer_SpacesPerKey(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	p := NewKeyedPacer(2*time.Second, 0, WithPacerClock(fc.Now))

	if !p.Allow("acctA") {
		t.Fatal("first request should be allowed")
	}
	fc.Advance(1 * time.Second) // still inside MinDelay
	if p.Allow("acctA") {
		t.Fatal("second request inside MinDelay must be denied")
	}
	fc.Advance(2 * time.Second) // now past MinDelay
	if !p.Allow("acctA") {
		t.Fatal("request after MinDelay elapsed must be allowed")
	}
}

// TestKeyedPacer_IndependentKeys verifies pacing is keyed by ACCOUNT — one
// account's recent request does NOT delay a different account. This is the
// core no-starvation property: N independent pacers, never one global gate.
func TestKeyedPacer_IndependentKeys(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	p := NewKeyedPacer(10*time.Second, 0, WithPacerClock(fc.Now))

	if !p.Allow("acctA") {
		t.Fatal("acctA first request should be allowed")
	}
	// acctB is a DIFFERENT key — must be allowed immediately even though acctA
	// just fired and is still well inside its MinDelay window.
	if !p.Allow("acctB") {
		t.Fatal("acctB must NOT be gated by acctA's recent request (per-account, not global)")
	}
	if !p.Allow("acctC") {
		t.Fatal("acctC must NOT be gated by acctA/acctB (per-account, not global)")
	}
}

// TestKeyedPacer_ZeroDelayDisabled verifies a zero MinDelay+RandomDelay pacer
// never gates (pacing disabled).
func TestKeyedPacer_ZeroDelayDisabled(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	p := NewKeyedPacer(0, 0, WithPacerClock(fc.Now))

	for i := 0; i < 5; i++ {
		if !p.Allow("acctA") {
			t.Fatalf("zero-delay pacer must never gate; denied at request %d", i+1)
		}
	}
}

// TestKeyedPacer_WaitReturnsWhenReady verifies Wait returns nil once the key is
// ready (does not block forever), using a real-clock pacer with a tiny delay.
func TestKeyedPacer_WaitReturnsWhenReady(t *testing.T) {
	p := NewKeyedPacer(20*time.Millisecond, 0) // real clock

	if err := p.Wait(context.Background(), "acctA"); err != nil {
		t.Fatalf("first Wait should return nil immediately: %v", err)
	}
	start := time.Now()
	if err := p.Wait(context.Background(), "acctA"); err != nil {
		t.Fatalf("second Wait should eventually return nil: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Fatalf("second Wait should have spaced ~20ms, only waited %v", elapsed)
	}
}

// TestKeyedPacer_JitterSampledOncePerGrant proves the random jitter is sampled
// exactly once when a request is granted (arming the next-allowed time), not
// re-rolled on every poll. With a fake clock there is no polling at all: the
// per-key next-allowed boundary is fixed at grant time, so a request lands
// exactly when the clock crosses it. We assert that across many grants the
// realized spacing actually varies (jitter is live) AND never drops below the
// MinDelay floor (the floor is honoured).
func TestKeyedPacer_JitterSampledOncePerGrant(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	const (
		minDelay = 100 * time.Millisecond
		rndDelay = 400 * time.Millisecond
	)
	p := NewKeyedPacer(minDelay, rndDelay, WithPacerClock(fc.Now))

	seen := map[time.Duration]struct{}{}
	prev := fc.now
	// Grant the first request, then for each subsequent grant advance the clock
	// in fine steps until the key is allowed again; the elapsed time IS the
	// sampled spacing for that grant.
	if !p.Allow("k") {
		t.Fatal("first grant must be allowed")
	}
	for g := 0; g < 30; g++ {
		steps := 0
		for !p.Allow("k") {
			fc.Advance(10 * time.Millisecond)
			steps++
			if steps > 100 {
				t.Fatal("key never became allowed; next-allowed not armed correctly")
			}
		}
		gap := fc.now.Sub(prev)
		prev = fc.now
		if gap < minDelay {
			t.Fatalf("realized spacing %v dropped below MinDelay floor %v", gap, minDelay)
		}
		if gap > minDelay+rndDelay+10*time.Millisecond {
			t.Fatalf("realized spacing %v exceeded MinDelay+RandomDelay band", gap)
		}
		seen[gap] = struct{}{}
	}
	// Jitter is live: spacing must take several distinct values, not a constant.
	if len(seen) < 5 {
		t.Fatalf("spacing not variable: only %d distinct gaps across 30 grants (jitter not live?)", len(seen))
	}
}

// TestKeyedPacer_WaitRespectsContext verifies Wait returns ctx.Err() on cancel.
func TestKeyedPacer_WaitRespectsContext(t *testing.T) {
	p := NewKeyedPacer(1*time.Hour, 0) // huge delay so Wait would block

	if err := p.Wait(context.Background(), "acctA"); err != nil {
		t.Fatalf("first Wait should return nil: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := p.Wait(ctx, "acctA"); err == nil {
		t.Fatal("second Wait under a 1h delay must return ctx error on timeout, got nil")
	}
}
