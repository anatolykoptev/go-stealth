package stealth

import (
	"testing"
	"time"
)

func TestBackoff_Duration(t *testing.T) {
	b := BackoffConfig{
		InitialWait: 1 * time.Second,
		MaxWait:     10 * time.Second,
		Multiplier:  2.0,
		JitterPct:   0, // no jitter for deterministic test
	}

	d0 := b.Duration(0)
	if d0 != 1*time.Second {
		t.Fatalf("attempt 0: expected 1s, got %v", d0)
	}

	d1 := b.Duration(1)
	if d1 != 2*time.Second {
		t.Fatalf("attempt 1: expected 2s, got %v", d1)
	}

	d2 := b.Duration(2)
	if d2 != 4*time.Second {
		t.Fatalf("attempt 2: expected 4s, got %v", d2)
	}

	// Should cap at MaxWait
	d10 := b.Duration(10)
	if d10 != 10*time.Second {
		t.Fatalf("attempt 10: expected 10s (max), got %v", d10)
	}
}

func TestBackoff_Jitter(t *testing.T) {
	b := BackoffConfig{
		InitialWait: 1 * time.Second,
		MaxWait:     10 * time.Second,
		Multiplier:  2.0,
		JitterPct:   0.5,
	}

	// Run multiple times to check jitter variation
	seen := make(map[time.Duration]bool)
	for i := 0; i < 20; i++ {
		d := b.Duration(0)
		seen[d] = true
		if d < 500*time.Millisecond || d > 1500*time.Millisecond {
			t.Fatalf("attempt 0 with 50%% jitter: got %v, expected 500ms-1500ms", d)
		}
	}
	if len(seen) < 2 {
		t.Fatal("jitter should produce varying durations")
	}
}
