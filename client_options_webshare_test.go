package stealth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// webshareProxyPage returns a minimal Webshare API JSON response with one proxy.
const webshareProxyPage = `{
	"results": [
		{"proxy_address": "1.2.3.4", "port": 10000, "username": "testuser-1", "password": "secret"}
	],
	"next": null
}`

func newWebshareTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestWithWebshareCountry_Default(t *testing.T) {
	var capturedURL string
	srv := newWebshareTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(webshareProxyPage))
	})

	_, err := NewClient(withWebshareCountryURL("test-key", srv.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if !strings.Contains(capturedURL, "country_code__in=US") {
		t.Fatalf("expected country_code__in=US in request URL, got: %s", capturedURL)
	}
}

func TestWithWebshareCountry_Multi(t *testing.T) {
	var capturedURL string
	srv := newWebshareTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(webshareProxyPage))
	})

	_, err := NewClient(withWebshareCountryURL("test-key", srv.URL, "US", "GB"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if !strings.Contains(capturedURL, "country_code__in=US%2CGB") &&
		!strings.Contains(capturedURL, "country_code__in=US,GB") {
		t.Fatalf("expected country_code__in=US,GB in request URL, got: %s", capturedURL)
	}
}

func TestWithWebshareRotating_NoAPICall(t *testing.T) {
	callCount := 0
	srv := newWebshareTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	})
	_ = srv // server registered but must not be called

	client, err := NewClient(
		WithWebshareRotating("myuser", "mypass", "US"),
		WithStdHTTP(), // avoid CGO tls-client in test
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if callCount != 0 {
		t.Fatalf("WithWebshareRotating must not make API calls; got %d call(s)", callCount)
	}
	// pool was set — verify proxy URL contains -US-rotate
	pool, ok := client.proxyPool.(interface{ Next() string })
	if !ok {
		t.Fatal("proxyPool not set")
	}
	next := pool.Next()
	if !strings.Contains(next, "-US-rotate") {
		t.Fatalf("expected -US-rotate in proxy URL, got: %s", next)
	}
}

func TestWithWebshareCountry_BuildError(t *testing.T) {
	// Empty API key → proxypool.NewWebshareWithConfig returns error → NewClient propagates it.
	_, err := NewClient(withWebshareCountryURL("", "http://irrelevant"))
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
}

// The client owns the pool WithWebshareCountry builds — Close must stop its
// credential refresher, and it must be idempotent.
func TestBrowserClient_Close_ClosesOwnedPool(t *testing.T) {
	var apiCalls atomic.Int64
	srv := newWebshareTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(webshareProxyPage))
	})

	client, err := NewClient(withWebshareCountryURL("test-key", srv.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	// The pool's refresher is stopped: a 407 report can no longer trigger a
	// credential fetch.
	before := apiCalls.Load()
	reporter, ok := client.proxyPool.(interface{ ReportAuthFailure() })
	if !ok {
		t.Fatal("pool does not implement ReportAuthFailure")
	}
	reporter.ReportAuthFailure()
	time.Sleep(100 * time.Millisecond)
	if got := apiCalls.Load(); got != before {
		t.Fatalf("credential fetch fired after Close: %d calls before, %d after", before, got)
	}
}

// closerPool records Close calls — a caller-supplied pool must NOT be closed
// by the client.
type closerPool struct {
	mockPool
	closed atomic.Int64
}

func (p *closerPool) Close() error { p.closed.Add(1); return nil }

func TestBrowserClient_Close_LeavesCallerPoolAlone(t *testing.T) {
	pool := &closerPool{mockPool: mockPool{proxies: []string{"http://u:p@1.2.3.4:8080"}}}
	client, err := NewClient(WithoutSSRFGuard(), WithProxyPool(pool))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := pool.closed.Load(); got != 0 {
		t.Fatalf("client must not close a caller-supplied pool, got %d closes", got)
	}
}
