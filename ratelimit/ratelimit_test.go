package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter_Allow(t *testing.T) {
	l := NewLimiter(Config{
		RequestsPerWindow: 3,
		WindowDuration:    1 * time.Minute,
	})

	for i := 0; i < 3; i++ {
		if !l.Allow("test") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	if l.Allow("test") {
		t.Fatal("4th request should be denied")
	}
}

func TestLimiter_SeparateKeys(t *testing.T) {
	l := NewLimiter(Config{
		RequestsPerWindow: 1,
		WindowDuration:    1 * time.Minute,
	})

	if !l.Allow("a") {
		t.Fatal("first request to 'a' should be allowed")
	}
	if !l.Allow("b") {
		t.Fatal("first request to 'b' should be allowed")
	}
	if l.Allow("a") {
		t.Fatal("second request to 'a' should be denied")
	}
}

func TestLimiter_MarkRateLimited(t *testing.T) {
	l := NewLimiter(Config{
		RequestsPerWindow: 100,
		WindowDuration:    1 * time.Minute,
	})

	l.MarkRateLimited("test", time.Now().Add(1*time.Hour))

	if l.Allow("test") {
		t.Fatal("should be blocked after MarkRateLimited")
	}
	if !l.IsRateLimited("test") {
		t.Fatal("should report as rate limited")
	}
}

func TestLimiter_AvailableAt(t *testing.T) {
	l := NewLimiter(Config{
		RequestsPerWindow: 1,
		WindowDuration:    5 * time.Minute,
	})

	at := l.AvailableAt("empty")
	if !at.IsZero() {
		t.Fatal("expected zero time for unused key")
	}

	l.Allow("used")
	at = l.AvailableAt("used")
	if at.IsZero() {
		t.Fatal("expected non-zero time after exhaustion")
	}
}
