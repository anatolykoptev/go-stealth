package proxypool

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ProxyHealth holds per-proxy health statistics.
type ProxyHealth struct {
	Successes    int64
	Failures     int64
	TotalLatency time.Duration
	LastUsed     time.Time
	DeactivateAt time.Time // zero = active
}

// SuccessRate returns the fraction of successful requests (0.0–1.0).
// Returns 1.0 if no requests have been recorded.
func (h *ProxyHealth) SuccessRate() float64 {
	total := h.Successes + h.Failures
	if total == 0 {
		return 1.0
	}
	return float64(h.Successes) / float64(total)
}

// AvgLatency returns average request latency.
func (h *ProxyHealth) AvgLatency() time.Duration {
	total := h.Successes + h.Failures
	if total == 0 {
		return 0
	}
	return h.TotalLatency / time.Duration(total)
}

// HealthyConfig configures the HealthyProxyPool wrapper.
type HealthyConfig struct {
	// FailureThreshold is the failure rate (0.0–1.0) at which a proxy is deactivated.
	// Default: 0.5 (50% failure rate).
	FailureThreshold float64

	// MinRequests is the minimum number of requests before health evaluation.
	// Prevents deactivation from a single failure. Default: 3.
	MinRequests int64

	// Cooldown is how long a deactivated proxy stays inactive before retrying.
	// Default: 5 minutes.
	Cooldown time.Duration
}

// DefaultHealthyConfig is suitable for most use cases.
var DefaultHealthyConfig = HealthyConfig{
	FailureThreshold: 0.5,
	MinRequests:      3,
	Cooldown:         5 * time.Minute,
}

// HealthyProxyPool wraps any ProxyPool with per-proxy health tracking.
// Dead proxies are skipped automatically and reactivated after a cooldown.
type HealthyProxyPool struct {
	pool    ProxyPool
	config  HealthyConfig
	mu      sync.RWMutex
	tracker map[string]*ProxyHealth
}

// NewHealthyPool wraps a ProxyPool with health tracking.
func NewHealthyPool(pool ProxyPool, cfg HealthyConfig) *HealthyProxyPool {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = DefaultHealthyConfig.FailureThreshold
	}
	if cfg.MinRequests <= 0 {
		cfg.MinRequests = DefaultHealthyConfig.MinRequests
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = DefaultHealthyConfig.Cooldown
	}
	return &HealthyProxyPool{
		pool:    pool,
		config:  cfg,
		tracker: make(map[string]*ProxyHealth),
	}
}

// Next returns the next healthy proxy. Skips deactivated proxies.
// Falls back to the underlying pool's Next() if all proxies are unhealthy.
func (hp *HealthyProxyPool) Next() string {
	poolLen := hp.pool.Len()
	now := time.Now()

	for i := 0; i < poolLen; i++ {
		proxy := hp.pool.Next()

		hp.mu.RLock()
		h, exists := hp.tracker[proxy]
		hp.mu.RUnlock()

		if !exists {
			return proxy
		}

		// Reactivate if cooldown expired
		if !h.DeactivateAt.IsZero() && now.After(h.DeactivateAt.Add(hp.config.Cooldown)) {
			hp.mu.Lock()
			h.DeactivateAt = time.Time{}
			h.Successes = 0
			h.Failures = 0
			hp.mu.Unlock()
			return proxy
		}

		// Skip deactivated
		if !h.DeactivateAt.IsZero() {
			continue
		}

		return proxy
	}

	// All proxies unhealthy — fall back to raw rotation
	return hp.pool.Next()
}

// Len delegates to the underlying pool.
func (hp *HealthyProxyPool) Len() int {
	return hp.pool.Len()
}

// TransportProxy returns a function suitable for http.Transport.Proxy using health-aware rotation.
func (hp *HealthyProxyPool) TransportProxy() func(*http.Request) (*url.URL, error) {
	return func(_ *http.Request) (*url.URL, error) {
		return url.Parse(hp.Next())
	}
}

// RecordSuccess records a successful request through the given proxy.
func (hp *HealthyProxyPool) RecordSuccess(proxy string, latency time.Duration) {
	hp.mu.Lock()
	defer hp.mu.Unlock()

	h := hp.getOrCreate(proxy)
	h.Successes++
	h.TotalLatency += latency
	h.LastUsed = time.Now()
}

// RecordFailure records a failed request through the given proxy.
// If the failure rate exceeds the threshold (after MinRequests), the proxy is deactivated.
func (hp *HealthyProxyPool) RecordFailure(proxy string, latency time.Duration) {
	hp.mu.Lock()
	defer hp.mu.Unlock()

	h := hp.getOrCreate(proxy)
	h.Failures++
	h.TotalLatency += latency
	h.LastUsed = time.Now()

	total := h.Successes + h.Failures
	if total >= hp.config.MinRequests && h.SuccessRate() < (1-hp.config.FailureThreshold) {
		h.DeactivateAt = time.Now()
	}
}

// Stats returns a snapshot of health stats for all tracked proxies.
func (hp *HealthyProxyPool) Stats() map[string]ProxyHealth {
	hp.mu.RLock()
	defer hp.mu.RUnlock()

	result := make(map[string]ProxyHealth, len(hp.tracker))
	for k, v := range hp.tracker {
		result[k] = *v
	}
	return result
}

// ActiveCount returns the number of currently active (non-deactivated) proxies.
func (hp *HealthyProxyPool) ActiveCount() int {
	hp.mu.RLock()
	defer hp.mu.RUnlock()

	active := hp.pool.Len()
	for _, h := range hp.tracker {
		if !h.DeactivateAt.IsZero() && time.Now().Before(h.DeactivateAt.Add(hp.config.Cooldown)) {
			active--
		}
	}
	if active < 0 {
		active = 0
	}
	return active
}

func (hp *HealthyProxyPool) getOrCreate(proxy string) *ProxyHealth {
	h, ok := hp.tracker[proxy]
	if !ok {
		h = &ProxyHealth{}
		hp.tracker[proxy] = h
	}
	return h
}
