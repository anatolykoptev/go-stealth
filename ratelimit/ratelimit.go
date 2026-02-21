package ratelimit

import (
	"sync"
	"time"
)

// Config defines rate limit parameters.
type Config struct {
	RequestsPerWindow int
	WindowDuration    time.Duration
}

// DefaultConfig is 50 requests per 15 minutes.
var DefaultConfig = Config{
	RequestsPerWindow: 50,
	WindowDuration:    15 * time.Minute,
}

// Limiter tracks per-key sliding window rate limits.
type Limiter struct {
	mu     sync.Mutex
	states map[string]*state
	config Config
}

type state struct {
	mu          sync.Mutex
	requests    int
	windowStart time.Time
	blockedUtil time.Time
}

// NewLimiter creates a rate limiter with the given config.
func NewLimiter(cfg Config) *Limiter {
	return &Limiter{
		states: make(map[string]*state),
		config: cfg,
	}
}

// Allow returns true if a request can be made for the given key.
// Atomically increments the counter when returning true.
func (l *Limiter) Allow(key string) bool {
	s := l.getState(key)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if now.Before(s.blockedUtil) {
		return false
	}

	if now.Sub(s.windowStart) > l.config.WindowDuration {
		s.requests = 0
		s.windowStart = now
	}

	if s.requests >= l.config.RequestsPerWindow {
		return false
	}

	s.requests++
	return true
}

// MarkRateLimited sets the blocked-until time for a key (e.g. from a 429 response).
func (l *Limiter) MarkRateLimited(key string, until time.Time) {
	s := l.getState(key)
	s.mu.Lock()
	s.blockedUtil = until
	s.mu.Unlock()
}

// IsRateLimited returns true if the key is currently blocked (read-only).
func (l *Limiter) IsRateLimited(key string) bool {
	s := l.getState(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if now.Before(s.blockedUtil) {
		return true
	}
	if s.requests >= l.config.RequestsPerWindow && now.Sub(s.windowStart) <= l.config.WindowDuration {
		return true
	}
	return false
}

// AvailableAt returns the time when the given key will become available.
// Returns zero time if available right now.
func (l *Limiter) AvailableAt(key string) time.Time {
	s := l.getState(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()

	var earliest time.Time

	if now.Before(s.blockedUtil) {
		earliest = s.blockedUtil
	}

	if s.requests >= l.config.RequestsPerWindow {
		windowEnd := s.windowStart.Add(l.config.WindowDuration)
		if now.Before(windowEnd) {
			if earliest.IsZero() || windowEnd.Before(earliest) {
				earliest = windowEnd
			}
		}
	}

	return earliest
}

func (l *Limiter) getState(key string) *state {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.states[key]
	if !ok {
		s = &state{windowStart: time.Now()}
		l.states[key] = s
	}
	return s
}
