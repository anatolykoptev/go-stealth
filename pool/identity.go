package pool

import (
	"sync"
	"time"
)

// Identity is the interface that pool items must implement.
type Identity interface {
	ID() string
	IsActive() bool
	SetActive(bool)
	ReactivateAt() time.Time
	SetReactivateAt(time.Time)
}

// HealthTracker provides thread-safe health tracking for pool items.
// Embed in your Identity implementation.
type HealthTracker struct {
	mu                  sync.Mutex
	totalRequests       int
	failedRequests      int
	consecutiveFailures int
	minRequests         int     // minimum requests before health check kicks in
	failThreshold       float64 // failure rate threshold for unhealthy
	maxConsecFailures   int     // auto-disable after N consecutive failures
	tripCount           int     // number of times this item has tripped the deactivation threshold
}

// NewHealthTracker creates a HealthTracker with the given thresholds.
func NewHealthTracker(minRequests int, failThreshold float64, maxConsecFailures int) HealthTracker {
	return HealthTracker{
		minRequests:       minRequests,
		failThreshold:     failThreshold,
		maxConsecFailures: maxConsecFailures,
	}
}

// DefaultHealthTracker creates a HealthTracker with sensible defaults.
func DefaultHealthTracker() HealthTracker {
	return NewHealthTracker(10, 0.8, 5)
}

// RecordSuccess records a successful request.
func (h *HealthTracker) RecordSuccess() {
	h.mu.Lock()
	h.totalRequests++
	h.consecutiveFailures = 0
	h.mu.Unlock()
}

// RecordFailure records a failed request. Returns true if the item
// should be deactivated (exceeded health thresholds).
func (h *HealthTracker) RecordFailure() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.totalRequests++
	h.failedRequests++
	h.consecutiveFailures++

	if h.consecutiveFailures >= h.maxConsecFailures {
		h.tripCount++
		return true
	}
	if h.totalRequests >= h.minRequests {
		rate := float64(h.failedRequests) / float64(h.totalRequests)
		if rate >= h.failThreshold {
			h.tripCount++
			return true
		}
		return false
	}
	return false
}

// Stats returns health counters.
func (h *HealthTracker) Stats() (total, failed, consecFails int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.totalRequests, h.failedRequests, h.consecutiveFailures
}

// TripCount returns how many times this item has tripped the deactivation
// threshold (consecutive-failure or failure-rate). Pass it to a backoff
// config so repeated trips against a flapping upstream grow the cooldown.
func (h *HealthTracker) TripCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tripCount
}

// Reset clears all failure counters.
func (h *HealthTracker) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.consecutiveFailures = 0
	h.failedRequests = 0
	h.totalRequests = 0
	h.tripCount = 0
}
