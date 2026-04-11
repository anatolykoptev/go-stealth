package stealth

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// setProxyFailBackend wraps a backend and makes SetProxy fail for the first N calls.
type setProxyFailBackend struct {
	inner      HTTPDoer
	failCount  int // number of leading SetProxy calls that fail
	callCount  atomic.Int64
	proxiesSet []string
}

func (b *setProxyFailBackend) Do(req *Request) (*Response, error) {
	return b.inner.Do(req)
}

func (b *setProxyFailBackend) SetProxy(url string) error {
	n := int(b.callCount.Add(1))
	if n <= b.failCount {
		return errors.New("mock SetProxy failure")
	}
	b.proxiesSet = append(b.proxiesSet, url)
	return nil
}

func (b *setProxyFailBackend) GetCookieValue(rawURL, name string) string {
	return b.inner.GetCookieValue(rawURL, name)
}

func newSetProxyFailFactory(b *setProxyFailBackend) BackendFactory {
	return func(_ BackendConfig) (HTTPDoer, error) {
		return b, nil
	}
}

// TestRetryOnBlock_SetProxyFailsThenSucceeds: pool has 3 proxies, SetProxy
// fails on the first two, succeeds on the third. Request must succeed using
// the third proxy, not the first.
func TestRetryOnBlock_SetProxyFailsThenSucceeds(t *testing.T) {
	t.Parallel()

	inner := &statusSequenceBackend{statuses: []int{http.StatusOK}}
	backend := &setProxyFailBackend{inner: inner, failCount: 2}
	pool := &mockPool{proxies: []string{"proxy1", "proxy2", "proxy3"}}

	bc, err := NewClient(
		WithBackend(newSetProxyFailFactory(backend)),
		WithProxyPool(pool),
		WithRetryOnBlock(2), // maxAttempts = 3
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, code, err := bc.Do(http.MethodGet, "https://example.com", nil, nil)
	if err != nil {
		t.Fatalf("expected success on third proxy, got error: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("want 200, got %d", code)
	}

	// SetProxy was called 3 times (2 failures + 1 success).
	if got := backend.callCount.Load(); got != 3 {
		t.Errorf("want 3 SetProxy calls, got %d", got)
	}
	// The successful proxy is proxy3 — the one set on the third attempt.
	if len(backend.proxiesSet) != 1 || backend.proxiesSet[0] != "proxy3" {
		t.Errorf("want proxy3 actually set, got %v", backend.proxiesSet)
	}
}

// TestRetryOnBlock_SetProxyAllFail: pool has 2 proxies, both SetProxy fail.
// doWithRetry must return an error indicating pool exhaustion.
func TestRetryOnBlock_SetProxyAllFail(t *testing.T) {
	t.Parallel()

	inner := &statusSequenceBackend{statuses: []int{http.StatusOK}}
	backend := &setProxyFailBackend{inner: inner, failCount: 10} // always fail
	pool := &mockPool{proxies: []string{"proxy1", "proxy2"}}

	bc, err := NewClient(
		WithBackend(newSetProxyFailFactory(backend)),
		WithProxyPool(pool),
		WithRetryOnBlock(1), // maxAttempts = 2
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = bc.Do(http.MethodGet, "https://example.com", nil, nil)
	if err == nil {
		t.Fatal("expected error when all SetProxy calls fail, got nil")
	}
	if !strings.Contains(err.Error(), "proxy pool exhausted") {
		t.Errorf("expected 'proxy pool exhausted' in error, got: %v", err)
	}

	// Confirm no actual HTTP request was made.
	if got := inner.call.Load(); got != 0 {
		t.Errorf("want 0 HTTP requests when all SetProxy fail, got %d", got)
	}
}
