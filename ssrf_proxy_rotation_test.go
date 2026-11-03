//go:build !race

package stealth

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

// Excluded under -race for the reason client_setproxy_concurrent_test.go
// gives: tls-client's SetProxy writes state that concurrent Do reads, which the
// race detector flags regardless of go-stealth's code. The make preflight
// non-race pass runs it and requires its PASS line.

// Concurrent rotation between two private proxies must never refuse the
// operator's own proxy: a request that picked P1 may dial after another
// goroutine switched to P2.
func TestLive_SSRFProxy_ConcurrentRotationNeverRefusesProxy(t *testing.T) {
	up := startTLSUpstream(t)
	backends := []struct {
		name string
		opts []ClientOption
	}{
		{"tls-client", nil},
		{"std", []ClientOption{WithStdHTTP()}},
	}
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			p1 := startTunnelProxy(t, up.Listener.Addr().String())
			p2 := startTunnelProxy(t, up.Listener.Addr().String())
			pool := &mockPool{proxies: []string{"http://" + p1.addr(), "http://" + p2.addr()}}
			opts := append([]ClientOption{
				WithProxyPool(pool), WithInsecureSkipVerify(), WithTimeout(10),
			}, b.opts...)
			bc, err := NewClient(opts...)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			t.Cleanup(func() { _ = bc.Close() })

			// buildHandler caches its chain without a lock (a separate defect
			// in the first concurrent Do); build it before the goroutines race.
			if _, _, _, err := bc.Do(http.MethodGet, "https://198.51.100.7/ok", nil, nil); err != nil {
				t.Fatalf("warm-up request: %v", err)
			}

			const workers, iters = 8, 20
			var blocked, other atomic.Int32
			var firstErr atomic.Value
			var wg sync.WaitGroup
			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < iters; i++ {
						_, _, _, err := bc.Do(http.MethodGet, "https://198.51.100.7/ok", nil, nil)
						switch {
						case err == nil:
						case errors.Is(err, ErrSSRFBlocked):
							blocked.Add(1)
							firstErr.CompareAndSwap(nil, err)
						default:
							other.Add(1)
						}
					}
				}()
			}
			wg.Wait()
			if n := blocked.Load(); n != 0 {
				t.Fatalf("%d/%d requests refused as SSRF-blocked during rotation; first: %v", n, workers*iters, firstErr.Load())
			}
			if n := other.Load(); n != 0 {
				t.Logf("%d requests failed for non-SSRF reasons (not asserted)", n)
			}
		})
	}
}
