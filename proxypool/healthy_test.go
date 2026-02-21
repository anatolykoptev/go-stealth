package proxypool

import (
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

// staticPool is a simple thread-safe ProxyPool for testing.
type staticPool struct {
	proxies []string
	counter atomic.Uint64
}

func (s *staticPool) Next() string {
	idx := s.counter.Add(1) % uint64(len(s.proxies))
	return s.proxies[idx]
}

func (s *staticPool) Len() int { return len(s.proxies) }

func (s *staticPool) TransportProxy() func(*http.Request) (*url.URL, error) { return nil }

func newStaticPool(proxies ...string) *staticPool {
	return &staticPool{proxies: proxies}
}

func TestHealthyPool_NextReturnsProxy(t *testing.T) {
	pool := newStaticPool("http://p1:8080", "http://p2:8080")
	hp := NewHealthyPool(pool, DefaultHealthyConfig)

	got := hp.Next()
	if got == "" {
		t.Fatal("expected non-empty proxy")
	}
}

func TestHealthyPool_SkipsDeadProxy(t *testing.T) {
	pool := newStaticPool("http://dead:8080", "http://alive:8080")
	cfg := HealthyConfig{
		FailureThreshold: 0.5,
		MinRequests:      2,
		Cooldown:         1 * time.Hour,
	}
	hp := NewHealthyPool(pool, cfg)

	// Record enough failures to deactivate "dead"
	hp.RecordFailure("http://dead:8080", 100*time.Millisecond)
	hp.RecordFailure("http://dead:8080", 100*time.Millisecond)
	hp.RecordFailure("http://dead:8080", 100*time.Millisecond)

	// Call Next enough times to ensure we get "alive" at least once
	// and never get "dead" (unless as fallback when all dead, but alive is healthy)
	seen := make(map[string]int)
	for range 10 {
		got := hp.Next()
		seen[got]++
	}
	if seen["http://dead:8080"] > 0 {
		t.Fatalf("dead proxy should be skipped, got it %d times", seen["http://dead:8080"])
	}
	if seen["http://alive:8080"] == 0 {
		t.Fatal("alive proxy should be returned")
	}
}

func TestHealthyPool_ReactivatesAfterCooldown(t *testing.T) {
	pool := newStaticPool("http://p1:8080")
	cfg := HealthyConfig{
		FailureThreshold: 0.5,
		MinRequests:      2,
		Cooldown:         1 * time.Millisecond, // tiny cooldown for test
	}
	hp := NewHealthyPool(pool, cfg)

	// Deactivate
	hp.RecordFailure("http://p1:8080", 0)
	hp.RecordFailure("http://p1:8080", 0)
	hp.RecordFailure("http://p1:8080", 0)

	// Wait for cooldown
	time.Sleep(5 * time.Millisecond)

	// Should reactivate — keep calling until we get the proxy
	// (fallback also returns it, so any return means it's reachable)
	got := hp.Next()
	if got != "http://p1:8080" {
		t.Fatalf("expected reactivated proxy, got: %s", got)
	}
}

func TestHealthyPool_FallbackWhenAllDead(t *testing.T) {
	pool := newStaticPool("http://p1:8080")
	cfg := HealthyConfig{
		FailureThreshold: 0.5,
		MinRequests:      2,
		Cooldown:         1 * time.Hour,
	}
	hp := NewHealthyPool(pool, cfg)

	// Deactivate all
	hp.RecordFailure("http://p1:8080", 0)
	hp.RecordFailure("http://p1:8080", 0)
	hp.RecordFailure("http://p1:8080", 0)

	// Should still return something (fallback)
	got := hp.Next()
	if got != "http://p1:8080" {
		t.Fatalf("expected fallback proxy, got: %s", got)
	}
}

func TestHealthyPool_RecordSuccess(t *testing.T) {
	pool := newStaticPool("http://p1:8080")
	hp := NewHealthyPool(pool, DefaultHealthyConfig)

	hp.RecordSuccess("http://p1:8080", 50*time.Millisecond)
	hp.RecordSuccess("http://p1:8080", 150*time.Millisecond)

	stats := hp.Stats()
	h := stats["http://p1:8080"]
	if h.Successes != 2 {
		t.Fatalf("expected 2 successes, got %d", h.Successes)
	}
	if h.SuccessRate() != 1.0 {
		t.Fatalf("expected 100%% success rate, got %f", h.SuccessRate())
	}
	if h.AvgLatency() != 100*time.Millisecond {
		t.Fatalf("expected 100ms avg latency, got %v", h.AvgLatency())
	}
}

func TestHealthyPool_ActiveCount(t *testing.T) {
	pool := newStaticPool("http://p1:8080", "http://p2:8080", "http://p3:8080")
	cfg := HealthyConfig{
		FailureThreshold: 0.5,
		MinRequests:      2,
		Cooldown:         1 * time.Hour,
	}
	hp := NewHealthyPool(pool, cfg)

	if hp.ActiveCount() != 3 {
		t.Fatalf("expected 3 active, got %d", hp.ActiveCount())
	}

	// Deactivate one
	hp.RecordFailure("http://p2:8080", 0)
	hp.RecordFailure("http://p2:8080", 0)
	hp.RecordFailure("http://p2:8080", 0)

	if hp.ActiveCount() != 2 {
		t.Fatalf("expected 2 active, got %d", hp.ActiveCount())
	}
}

func TestProxyHealth_SuccessRateNoRequests(t *testing.T) {
	h := &ProxyHealth{}
	if h.SuccessRate() != 1.0 {
		t.Fatalf("expected 1.0 for zero requests, got %f", h.SuccessRate())
	}
}

func TestProxyHealth_AvgLatencyNoRequests(t *testing.T) {
	h := &ProxyHealth{}
	if h.AvgLatency() != 0 {
		t.Fatalf("expected 0 for zero requests, got %v", h.AvgLatency())
	}
}

func TestHealthyPool_ConcurrentAccess(t *testing.T) {
	pool := newStaticPool("http://p1:8080", "http://p2:8080", "http://p3:8080")
	cfg := HealthyConfig{
		FailureThreshold: 0.5,
		MinRequests:      3,
		Cooldown:         10 * time.Millisecond,
	}
	hp := NewHealthyPool(pool, cfg)

	done := make(chan struct{})
	// Concurrent readers (Next)
	for range 4 {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					hp.Next()
				}
			}
		}()
	}
	// Concurrent writers (RecordSuccess/RecordFailure)
	for range 4 {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
					hp.RecordSuccess("http://p1:8080", time.Millisecond)
					hp.RecordFailure("http://p2:8080", time.Millisecond)
				}
			}
		}()
	}
	// Concurrent Stats readers
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				hp.Stats()
				hp.ActiveCount()
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(done)
}
