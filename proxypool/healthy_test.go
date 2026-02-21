package proxypool

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

// staticPool is a simple ProxyPool for testing.
type staticPool struct {
	proxies []string
	idx     int
}

func (s *staticPool) Next() string {
	p := s.proxies[s.idx%len(s.proxies)]
	s.idx++
	return p
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
	if got != "http://p1:8080" && got != "http://p2:8080" {
		t.Fatalf("unexpected proxy: %s", got)
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

	// Next should skip "dead" and return "alive"
	got := hp.Next()
	if got != "http://alive:8080" {
		t.Fatalf("expected alive proxy, got: %s", got)
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

	// Should reactivate
	got := hp.Next()
	if got != "http://p1:8080" {
		t.Fatalf("expected reactivated proxy, got: %s", got)
	}

	// Stats should be reset
	stats := hp.Stats()
	h := stats["http://p1:8080"]
	if h.Successes != 0 || h.Failures != 0 {
		t.Fatalf("expected reset stats after reactivation, got: s=%d f=%d", h.Successes, h.Failures)
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
