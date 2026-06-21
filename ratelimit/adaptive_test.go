package ratelimit

import (
	"testing"
	"time"
)

// fakeClock is a deterministic, injectable clock for rate-limit tests.
// It lets tests advance time without time.Sleep so the suite stays fast
// and the sliding-window / reset behaviour is exercised precisely.
type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time { return f.now }
func (f *fakeClock) Advance(d time.Duration) {
	f.now = f.now.Add(d)
}

// TestLimiter_WithClock verifies the injected clock drives window expiry,
// replacing the time.Sleep pattern with deterministic advance.
func TestLimiter_WithClock(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	l := NewLimiter(
		Config{RequestsPerWindow: 2, WindowDuration: time.Minute},
		WithClock(fc.Now),
	)

	if !l.Allow("k") || !l.Allow("k") {
		t.Fatal("first 2 requests should be allowed")
	}
	if l.Allow("k") {
		t.Fatal("3rd request within the window should be denied")
	}

	// Advance past the window — counter should reset.
	fc.Advance(2 * time.Minute)
	if !l.Allow("k") {
		t.Fatal("request after window expiry should be allowed")
	}
}

// TestLimiter_UpdateLimit_RaisesCap verifies that an adaptive update from an
// x-rate-limit-limit header raises a key's effective cap above the static
// Config cap (the core adaptive behaviour: X says the real limit is higher).
func TestLimiter_UpdateLimit_RaisesCap(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	l := NewLimiter(
		Config{RequestsPerWindow: 50, WindowDuration: 15 * time.Minute},
		WithClock(fc.Now),
	)

	// X header says the real per-account-per-endpoint limit is 187 (Followers).
	l.UpdateLimit("Followers", 187)

	// 51 requests must now be allowed (would fail at the static 50 cap).
	for i := 0; i < 51; i++ {
		if !l.Allow("Followers") {
			t.Fatalf("request %d should be allowed after UpdateLimit to 187 (static cap was 50)", i+1)
		}
	}
}

// TestLimiter_UpdateLimit_LowersCap verifies an adaptive update can also LOWER
// the cap (X reduced the limit for this account/endpoint) — self-tuning down.
func TestLimiter_UpdateLimit_LowersCap(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	l := NewLimiter(
		Config{RequestsPerWindow: 500, WindowDuration: 15 * time.Minute},
		WithClock(fc.Now),
	)

	l.UpdateLimit("UserTweets", 5)

	allowed := 0
	for i := 0; i < 10; i++ {
		if l.Allow("UserTweets") {
			allowed++
		}
	}
	if allowed != 5 {
		t.Fatalf("expected exactly 5 allowed after UpdateLimit to 5, got %d", allowed)
	}
}

// TestLimiter_UpdateLimit_PerKey verifies that an adaptive update on one key
// does NOT affect another key — caps are independent per endpoint.
func TestLimiter_UpdateLimit_PerKey(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	l := NewLimiter(
		Config{RequestsPerWindow: 50, WindowDuration: 15 * time.Minute},
		WithClock(fc.Now),
	)

	l.UpdateLimit("Retweeters", 500)

	// Retweeters got 500, UserTweets keeps the static 50.
	utAllowed := 0
	for i := 0; i < 60; i++ {
		if l.Allow("UserTweets") {
			utAllowed++
		}
	}
	if utAllowed != 50 {
		t.Fatalf("UserTweets should keep static cap 50, got %d", utAllowed)
	}
}

// TestLimiter_UpdateLimit_NonPositiveIgnored verifies a non-positive limit
// (absent / malformed header) is ignored and the prior cap remains — never
// collapses the key to deny-everything.
func TestLimiter_UpdateLimit_NonPositiveIgnored(t *testing.T) {
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	l := NewLimiter(
		Config{RequestsPerWindow: 10, WindowDuration: time.Minute},
		WithClock(fc.Now),
	)

	l.UpdateLimit("k", 0)  // malformed header → ignore
	l.UpdateLimit("k", -1) // malformed header → ignore

	allowed := 0
	for i := 0; i < 15; i++ {
		if l.Allow("k") {
			allowed++
		}
	}
	if allowed != 10 {
		t.Fatalf("non-positive UpdateLimit must be ignored, expected static cap 10, got %d", allowed)
	}
}
