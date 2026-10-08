package stealth

import (
	"net/http"
	"sync/atomic"
	"testing"
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
