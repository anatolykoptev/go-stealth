package proxypool

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// rotatingWebshare is a fake Webshare API whose proxy password can be rotated
// and whose failure mode can be toggled. It records how many list requests it
// served so tests can assert exactly-one-refresh semantics.
type rotatingWebshare struct {
	srv *httptest.Server

	mu       sync.Mutex
	password string
	status   int
	calls    atomic.Int64
}

func newRotatingWebshare(t *testing.T, password string) *rotatingWebshare {
	t.Helper()
	f := &rotatingWebshare{password: password, status: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.mu.Lock()
		pw, st := f.password, f.status
		f.mu.Unlock()
		if st != http.StatusOK {
			w.WriteHeader(st)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"results":[{"proxy_address":"1.2.3.4","port":8080,"username":"u","password":%q}],"next":null}`, pw)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *rotatingWebshare) rotate(password string) {
	f.mu.Lock()
	f.password = password
	f.mu.Unlock()
}

func (f *rotatingWebshare) failWith(status int) {
	f.mu.Lock()
	f.status = status
	f.mu.Unlock()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// Periodic refresh must swap in the new credentials after the API side rotates
// the proxy password — the issue's core scenario.
func TestWebshare_PeriodicRefresh_PicksUpRotatedPassword(t *testing.T) {
	fake := newRotatingWebshare(t, "old-pass")

	pool, err := NewWebshareWithConfig("test-key", WebshareConfig{
		BaseURL:         fake.srv.URL,
		RefreshInterval: 30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewWebshareWithConfig: %v", err)
	}
	defer func() { _ = pool.Close() }()

	if got := pool.Next(); !strings.Contains(got, "old-pass") {
		t.Fatalf("expected initial password in proxy URL, got %s", got)
	}

	fake.rotate("new-pass")

	waitFor(t, "pool to hand out rotated credentials", func() bool {
		return strings.Contains(pool.Next(), "new-pass")
	})

	if got := pool.Stats().RefreshSuccesses; got < 1 {
		t.Fatalf("expected at least one successful refresh, got %d", got)
	}
}

// A burst of concurrent 407 reports must coalesce into exactly one credential
// refresh (single-flight + minimum gap).
func TestWebshare_ReportAuthFailure_SingleRefreshUnderBurst(t *testing.T) {
	fake := newRotatingWebshare(t, "pass")

	pool, err := NewWebshareWithConfig("test-key", WebshareConfig{
		BaseURL:         fake.srv.URL,
		RefreshInterval: -1, // periodic refresh off — only the 407 path may fire
		RefreshMinGap:   time.Hour,
	})
	if err != nil {
		t.Fatalf("NewWebshareWithConfig: %v", err)
	}
	defer func() { _ = pool.Close() }()

	callsBefore := fake.calls.Load()

	const reporters = 20
	var wg sync.WaitGroup
	wg.Add(reporters)
	for range reporters {
		go func() {
			defer wg.Done()
			pool.ReportAuthFailure()
		}()
	}
	wg.Wait()

	waitFor(t, "triggered refresh to complete", func() bool {
		return pool.Stats().RefreshSuccesses == 1
	})

	// One more report inside the min gap must still not trigger a fetch.
	pool.ReportAuthFailure()
	time.Sleep(100 * time.Millisecond)

	if got := fake.calls.Load() - callsBefore; got != 1 {
		t.Fatalf("expected exactly 1 refresh fetch for the whole burst, got %d", got)
	}
	if got := pool.Stats().AuthFailures; got != reporters+1 {
		t.Fatalf("expected %d auth failures counted, got %d", reporters+1, got)
	}
}

// A failed refresh must keep serving the previous list — never empty the pool.
func TestWebshare_RefreshFailure_KeepsOldList(t *testing.T) {
	fake := newRotatingWebshare(t, "keep-me")

	pool, err := NewWebshareWithConfig("test-key", WebshareConfig{
		BaseURL:         fake.srv.URL,
		RefreshInterval: -1,
	})
	if err != nil {
		t.Fatalf("NewWebshareWithConfig: %v", err)
	}
	defer func() { _ = pool.Close() }()

	fake.failWith(http.StatusInternalServerError)
	pool.ReportAuthFailure()

	waitFor(t, "refresh failure to be counted", func() bool {
		return pool.Stats().RefreshFailures == 1
	})

	if got := pool.Next(); !strings.Contains(got, "keep-me") {
		t.Fatalf("expected old credentials to survive a failed refresh, got %s", got)
	}
	if got := pool.Len(); got != 1 {
		t.Fatalf("pool must not shrink after a failed refresh, len=%d", got)
	}
	if got := pool.Stats().RefreshSuccesses; got != 0 {
		t.Fatalf("expected 0 successful refreshes, got %d", got)
	}
}

// Close must stop the periodic refresher — no goroutine leak.
func TestWebshare_Close_StopsRefresher(t *testing.T) {
	fake := newRotatingWebshare(t, "pass")

	pool, err := NewWebshareWithConfig("test-key", WebshareConfig{
		BaseURL:         fake.srv.URL,
		RefreshInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewWebshareWithConfig: %v", err)
	}

	waitFor(t, "first periodic refresh", func() bool {
		return pool.Stats().RefreshSuccesses >= 1
	})

	if err := pool.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	callsAtClose := fake.calls.Load()
	time.Sleep(100 * time.Millisecond) // ~5 refresh intervals
	if got := fake.calls.Load(); got != callsAtClose {
		t.Fatalf("refresher kept fetching after Close: %d calls at close, %d after", callsAtClose, got)
	}

	// Close must be idempotent.
	if err := pool.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// Close must cancel an in-flight credential fetch: the refresher context is
// wired into the API request, so a hung Webshare API cannot wedge Close.
func TestWebshare_Close_CancelsInFlightFetch(t *testing.T) {
	var apiCalls atomic.Int64
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiCalls.Add(1) > 1 {
			once.Do(func() { close(fetchStarted) })
			select {
			case <-releaseFetch:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"proxy_address":"1.2.3.4","port":8080,"username":"u","password":"p"}],"next":null}`))
	}))
	t.Cleanup(srv.Close)

	pool, err := NewWebshareWithConfig("test-key", WebshareConfig{
		BaseURL:         srv.URL,
		RefreshInterval: -1,
		RefreshMinGap:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewWebshareWithConfig: %v", err)
	}

	pool.ReportAuthFailure()
	<-fetchStarted

	closed := make(chan struct{})
	go func() {
		_ = pool.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		close(releaseFetch) // unwind so httptest cleanup cannot hang
		t.Fatal("Close blocked on an in-flight credential fetch — the fetch ignores the refresher context")
	}
}

// A 407 trigger arriving while a refresh is already in flight must join the
// in-flight fetch (singleflight), not start a second one. This is the part
// min-gap alone cannot cover: the first trigger always passes the gap check.
func TestWebshare_TriggeredRefresh_CoalescesWithInFlightFetch(t *testing.T) {
	var apiCalls atomic.Int64
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiCalls.Add(1) > 1 {
			once.Do(func() { close(fetchStarted) })
			select {
			case <-releaseFetch:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"proxy_address":"1.2.3.4","port":8080,"username":"u","password":"p"}],"next":null}`))
	}))
	t.Cleanup(srv.Close)

	pool, err := NewWebshareWithConfig("test-key", WebshareConfig{
		BaseURL:         srv.URL,
		RefreshInterval: -1,
		RefreshMinGap:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewWebshareWithConfig: %v", err)
	}
	callsBefore := apiCalls.Load()

	refreshDone := make(chan struct{})
	go func() {
		pool.refresh("periodic")
		close(refreshDone)
	}()
	<-fetchStarted

	pool.ReportAuthFailure()
	// Give the trigger goroutine a scheduling margin so its sf.Do lands while
	// the fetch above is still blocked — otherwise it may start a second call
	// legitimately after the first completes.
	time.Sleep(100 * time.Millisecond)
	close(releaseFetch)
	<-refreshDone

	waitFor(t, "coalesced refresh to complete", func() bool {
		return pool.Stats().RefreshSuccesses >= 1
	})
	if err := pool.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := apiCalls.Load() - callsBefore; got != 1 {
		t.Fatalf("407 trigger during in-flight refresh must coalesce into 1 fetch, got %d", got)
	}
}

// Pools built without API access (rotating creds, no key) cannot refresh —
// ReportAuthFailure must still count safely without panicking.
func TestWebshareRotating_ReportAuthFailure_NoRefreshPath(t *testing.T) {
	pool, err := NewWebshareRotating("user", "pass", "US")
	if err != nil {
		t.Fatalf("NewWebshareRotating: %v", err)
	}

	pool.ReportAuthFailure()
	if got := pool.Stats().AuthFailures; got != 1 {
		t.Fatalf("expected 1 auth failure counted, got %d", got)
	}
	if err := pool.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// HealthyProxyPool must forward ReportAuthFailure to a wrapped pool that
// supports it, so wrapping a Webshare pool does not silently drop the 407 path.
type recordingPool struct {
	proxies  []string
	reported atomic.Int64
}

func (p *recordingPool) Next() string { return p.proxies[0] }
func (p *recordingPool) Len() int     { return len(p.proxies) }
func (p *recordingPool) TransportProxy() func(*http.Request) (*url.URL, error) {
	return nil
}
func (p *recordingPool) ReportAuthFailure() { p.reported.Add(1) }

func TestHealthyPool_ForwardsReportAuthFailure(t *testing.T) {
	inner := &recordingPool{proxies: []string{"http://u:p@1.2.3.4:8080"}}
	hp := NewHealthyPool(inner, DefaultHealthyConfig)

	hp.ReportAuthFailure()
	if got := inner.reported.Load(); got != 1 {
		t.Fatalf("expected forwarded auth-failure report, got %d", got)
	}
}
