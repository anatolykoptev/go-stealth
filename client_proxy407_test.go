package stealth

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-stealth/proxypool"
)

// reportingPool records ReportAuthFailure calls — stands in for
// *proxypool.Webshare in the client-level 407 path.
type reportingPool struct {
	mockPool
	reported atomic.Int64
}

func (p *reportingPool) ReportAuthFailure() { p.reported.Add(1) }

// A 407 (Proxy Authentication Required) response must be reported to the pool
// so it can refresh credentials, and must still be returned to the caller.
func TestDo_407_ReportsAuthFailure(t *testing.T) {
	t.Parallel()
	backend := &statusSequenceBackend{statuses: []int{http.StatusProxyAuthRequired}}
	pool := &reportingPool{mockPool: mockPool{proxies: []string{"http://u:p@1.2.3.4:8080"}}}

	bc, err := NewClient(WithoutSSRFGuard(),
		WithBackend(newTestBackendFactory(backend)),
		WithProxyPool(pool),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, code, err := bc.Do("GET", "https://example.com", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusProxyAuthRequired {
		t.Fatalf("want 407 returned to caller, got %d", code)
	}
	if got := pool.reported.Load(); got != 1 {
		t.Fatalf("want 1 auth-failure report to the pool, got %d", got)
	}
}

// On a real backend a proxy 407 is a CONNECT rejection that surfaces as a
// transport ERROR, not a response — the resp.StatusCode == 407 path alone
// never fires for it. A local httptest proxy answers 407 to CONNECT (and to
// absolute-form plain-HTTP requests); the pool is a real *proxypool.Webshare
// built against a fake Webshare API, so the report must also drive a real
// credential re-fetch (API call 2+).
func TestDo_407TransportError_ReportsAuthFailure(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		std     bool   // WithStdHTTP; false = default tls-client backend
		target  string // https:// = CONNECT tunnel
		wantErr bool   // CONNECT 407 arrives as error; absolute-form 407 as a response
	}{
		{"tls-client https", false, "https://example.com/", true},
		{"tls-client http", false, "http://example.com/", true},
		{"std https", true, "https://example.com/", true},
		{"std http", true, "http://example.com/", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusProxyAuthRequired)
			}))
			t.Cleanup(proxy.Close)
			proxyPort := proxy.Listener.Addr().(*net.TCPAddr).Port

			var apiCalls atomic.Int64
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				apiCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"results":[{"proxy_address":"127.0.0.1","port":%d,"username":"u","password":"p"}],"next":null}`, proxyPort)
			}))
			t.Cleanup(api.Close)

			pool, err := proxypool.NewWebshareWithConfig("test-key", proxypool.WebshareConfig{
				BaseURL:         api.URL,
				RefreshInterval: -1, // periodic off — only the 407 trigger may fire
				RefreshMinGap:   time.Millisecond,
			})
			if err != nil {
				t.Fatalf("NewWebshareWithConfig: %v", err)
			}
			t.Cleanup(func() { _ = pool.Close() })

			opts := []ClientOption{WithoutSSRFGuard(), WithProxyPool(pool)}
			if tc.std {
				opts = append(opts, WithStdHTTP())
			}
			bc, err := NewClient(opts...)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			_, _, code, err := bc.Do("GET", tc.target, nil, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected CONNECT-407 transport error, got status %d", code)
				}
				t.Logf("transport error: %v", err)
			} else {
				if err != nil {
					t.Fatalf("unexpected transport error: %v", err)
				}
				if code != http.StatusProxyAuthRequired {
					t.Fatalf("want 407 response, got %d", code)
				}
			}

			if got := pool.Stats().AuthFailures; got != 1 {
				t.Fatalf("want 1 auth-failure report to the pool, got %d", got)
			}
			waitForClient(t, "407-triggered credential refresh", func() bool {
				return apiCalls.Load() >= 2
			})
		})
	}
}

// waitForClient polls cond until it holds or the deadline passes.
func waitForClient(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A pool without ReportAuthFailure support must not break the 407 path.
func TestDo_407_PoolWithoutReporting(t *testing.T) {
	t.Parallel()
	backend := &statusSequenceBackend{statuses: []int{http.StatusProxyAuthRequired}}
	pool := &mockPool{proxies: []string{"http://u:p@1.2.3.4:8080"}}

	bc, err := NewClient(WithoutSSRFGuard(),
		WithBackend(newTestBackendFactory(backend)),
		WithProxyPool(pool),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, _, code, err := bc.Do("GET", "https://example.com", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusProxyAuthRequired {
		t.Fatalf("want 407, got %d", code)
	}
}
