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
