package stealth

import (
	"net/http"
	"sync/atomic"
	"testing"
)

// mockPool cycles through a list of proxy URLs.
type mockPool struct {
	proxies []string
	idx     atomic.Int64
}

func (p *mockPool) Next() string {
	i := p.idx.Add(1) - 1
	return p.proxies[i%int64(len(p.proxies))]
}

// statusSequenceBackend returns status codes from the given sequence.
type statusSequenceBackend struct {
	statuses []int
	call     atomic.Int64
	proxies  []string // records proxy URLs set via SetProxy
}

func (b *statusSequenceBackend) Do(req *Request) (*Response, error) {
	i := b.call.Add(1) - 1
	code := b.statuses[i%int64(len(b.statuses))]
	return &Response{StatusCode: code, Body: []byte("ok"), Headers: map[string]string{}}, nil
}

func (b *statusSequenceBackend) SetProxy(url string) error {
	b.proxies = append(b.proxies, url)
	return nil
}

func (b *statusSequenceBackend) GetCookieValue(_, _ string) string { return "" }

func newTestBackendFactory(b *statusSequenceBackend) BackendFactory {
	return func(_ BackendConfig) (HTTPDoer, error) {
		return b, nil
	}
}

func TestRetryOnBlock_403ThenOK(t *testing.T) {
	t.Parallel()
	backend := &statusSequenceBackend{statuses: []int{http.StatusForbidden, http.StatusOK}}
	pool := &mockPool{proxies: []string{"proxy1", "proxy2", "proxy3"}}

	bc, err := NewClient(
		WithBackend(newTestBackendFactory(backend)),
		WithProxyPool(pool),
		WithRetryOnBlock(2),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, code, err := bc.Do("GET", "https://example.com", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusOK {
		t.Errorf("want 200, got %d", code)
	}
	if got := backend.call.Load(); got != 2 {
		t.Errorf("want 2 attempts, got %d", got)
	}
	// Should have used 2 different proxies.
	if len(backend.proxies) != 2 {
		t.Errorf("want 2 proxy rotations, got %d", len(backend.proxies))
	}
	if backend.proxies[0] == backend.proxies[1] {
		t.Error("proxy should rotate between retries")
	}
}

func TestRetryOnBlock_NoPoolNoRetry(t *testing.T) {
	t.Parallel()
	backend := &statusSequenceBackend{statuses: []int{http.StatusForbidden}}

	bc, err := NewClient(
		WithBackend(newTestBackendFactory(backend)),
		WithRetryOnBlock(2),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, code, err := bc.Do("GET", "https://example.com", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusForbidden {
		t.Errorf("want 403 without pool, got %d", code)
	}
	if got := backend.call.Load(); got != 1 {
		t.Errorf("want 1 attempt without pool, got %d", got)
	}
}

func TestRetryOnBlock_200NoRetry(t *testing.T) {
	t.Parallel()
	backend := &statusSequenceBackend{statuses: []int{http.StatusOK}}
	pool := &mockPool{proxies: []string{"proxy1"}}

	bc, err := NewClient(
		WithBackend(newTestBackendFactory(backend)),
		WithProxyPool(pool),
		WithRetryOnBlock(2),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, code, err := bc.Do("GET", "https://example.com", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusOK {
		t.Errorf("want 200, got %d", code)
	}
	if got := backend.call.Load(); got != 1 {
		t.Errorf("want 1 attempt on success, got %d", got)
	}
}

func TestRetryOnBlock_AllFail(t *testing.T) {
	t.Parallel()
	backend := &statusSequenceBackend{statuses: []int{
		http.StatusForbidden, http.StatusForbidden, http.StatusForbidden,
	}}
	pool := &mockPool{proxies: []string{"p1", "p2", "p3"}}

	bc, err := NewClient(
		WithBackend(newTestBackendFactory(backend)),
		WithProxyPool(pool),
		WithRetryOnBlock(2),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, code, err := bc.Do("GET", "https://example.com", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusForbidden {
		t.Errorf("want 403 after exhausting retries, got %d", code)
	}
	if got := backend.call.Load(); got != 3 {
		t.Errorf("want 3 total attempts (1 + 2 retries), got %d", got)
	}
}

func TestRetryOnBlock_429(t *testing.T) {
	t.Parallel()
	backend := &statusSequenceBackend{statuses: []int{
		http.StatusTooManyRequests, http.StatusOK,
	}}
	pool := &mockPool{proxies: []string{"p1", "p2"}}

	bc, err := NewClient(
		WithBackend(newTestBackendFactory(backend)),
		WithProxyPool(pool),
		WithRetryOnBlock(1),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, code, err := bc.Do("GET", "https://example.com", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusOK {
		t.Errorf("want 200 after 429 retry, got %d", code)
	}
}

func TestIsBlockStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code int
		want bool
	}{
		{403, true},
		{429, true},
		{502, true},
		{503, true},
		{200, false},
		{500, false},
		{404, false},
	}
	for _, tt := range tests {
		if got := isBlockStatus(tt.code); got != tt.want {
			t.Errorf("isBlockStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}
