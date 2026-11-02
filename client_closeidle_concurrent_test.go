//go:build !race

package stealth

// Excluded under -race for the same reason as the proxy rotation test: the
// tls-client transport has an upstream unsynchronised access (see
// client_setproxy_concurrent_test.go). It runs in the non-race preflight pass.

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// CloseIdleConnections beside running requests must not fail them (Close
// would drop the cached transport and fail concurrent h1 dials).
func TestLive_CloseIdleConnections_ConcurrentWithDo(t *testing.T) {
	for _, h2 := range []bool{false, true} {
		name := "h1"
		if h2 {
			name = "h2"
		}
		t.Run(name, func(t *testing.T) {
			var served atomic.Int64
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				served.Add(1)
				time.Sleep(15 * time.Millisecond)
				_, _ = w.Write([]byte("ok"))
			}))
			srv.EnableHTTP2 = h2
			srv.StartTLS()
			t.Cleanup(srv.Close)

			bc, err := NewClient(WithoutSSRFGuard(), WithInsecureSkipVerify())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = bc.Close() })

			stop := make(chan struct{})
			var calls atomic.Int64
			var rot sync.WaitGroup
			rot.Add(1)
			go func() {
				defer rot.Done()
				for {
					select {
					case <-stop:
						return
					case <-time.After(5 * time.Millisecond):
						bc.CloseIdleConnections()
						calls.Add(1)
					}
				}
			}()

			var wg sync.WaitGroup
			var bad atomic.Int64
			var firstErr atomic.Value
			for g := 0; g < 8; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < 25; i++ {
						if _, _, code, err := bc.Do("GET", srv.URL, nil, nil); err != nil || code != 200 {
							bad.Add(1)
							firstErr.CompareAndSwap(nil, err)
						}
					}
				}()
			}
			wg.Wait()
			close(stop)
			rot.Wait()

			if bad.Load() != 0 {
				t.Fatalf("%d of 200 requests failed beside CloseIdleConnections, want 0; first: %v", bad.Load(), firstErr.Load())
			}
			if calls.Load() < 5 || served.Load() != 200 {
				t.Fatalf("not exercised: %d CloseIdleConnections calls, %d requests served (want >=5, 200)", calls.Load(), served.Load())
			}
		})
	}
}
