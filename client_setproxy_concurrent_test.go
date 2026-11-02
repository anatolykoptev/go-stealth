//go:build !race

package stealth

// Excluded under -race: tls-client's httpClient.SetProxy writes c.Transport
// while Do reads it, an unsynchronised access that exists upstream (and on
// origin/main), so any test that rotates the proxy during live traffic trips
// the race detector regardless of go-stealth's code. The race is not
// suppressed here; the test simply runs in the non-race pass only.

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// startConnectProxy runs a minimal HTTP CONNECT proxy and counts tunnels.
func startConnectProxy(t *testing.T) (url string, tunnels *atomic.Int64) {
	t.Helper()
	tunnels = &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "connect only", http.StatusMethodNotAllowed)
			return
		}
		up, err := net.DialTimeout("tcp", r.Host, 5*time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			_ = up.Close()
			return
		}
		down, _, err := hj.Hijack()
		if err != nil {
			_ = up.Close()
			return
		}
		tunnels.Add(1)
		_, _ = down.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		go func() { _, _ = io.Copy(up, down); _ = up.Close() }()
		go func() { _, _ = io.Copy(down, up); _ = down.Close() }()
	}))
	t.Cleanup(srv.Close)
	return srv.URL, tunnels
}

// Rotating the proxy while requests are in flight must not fail them.
// SetProxy used to Close() the tls-client, dropping its cached transport under
// concurrent dials (errProtocolChanged on h1): 30-38% of requests failed.
func TestLive_SetProxyRotation_DoesNotFailInFlightRequests(t *testing.T) {
	for _, h2 := range []bool{false, true} {
		name := "h1"
		if h2 {
			name = "h2"
		}
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(15 * time.Millisecond)
				_, _ = w.Write([]byte("ok"))
			}))
			srv.EnableHTTP2 = h2
			srv.StartTLS()
			t.Cleanup(srv.Close)

			p1, n1 := startConnectProxy(t)
			p2, n2 := startConnectProxy(t)
			bc, err := NewClient(WithoutSSRFGuard(), WithInsecureSkipVerify(), WithProxy(p1))
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			t.Cleanup(func() { _ = bc.Close() })

			stop := make(chan struct{})
			var rot sync.WaitGroup
			rot.Add(1)
			go func() {
				defer rot.Done()
				proxies := []string{p2, p1}
				for i := 0; ; i++ {
					select {
					case <-stop:
						return
					case <-time.After(5 * time.Millisecond):
						_ = bc.SetProxy(proxies[i%2])
					}
				}
			}()

			var wg sync.WaitGroup
			var ok, bad atomic.Int64
			var firstErr atomic.Value
			for g := 0; g < 8; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < 25; i++ {
						if _, _, code, err := bc.Do("GET", srv.URL, nil, nil); err != nil || code != 200 {
							bad.Add(1)
							firstErr.CompareAndSwap(nil, fmt.Sprintf("code=%d err=%v", code, err))
							continue
						}
						ok.Add(1)
					}
				}()
			}
			wg.Wait()
			close(stop)
			rot.Wait()

			if n1.Load() == 0 || n2.Load() == 0 {
				t.Fatalf("traffic did not cross both proxies (proxy1=%d proxy2=%d tunnels): rotation not exercised", n1.Load(), n2.Load())
			}
			if bad.Load() != 0 {
				t.Fatalf("%d of %d requests failed during proxy rotation, want 0; first: %v", bad.Load(), ok.Load()+bad.Load(), firstErr.Load())
			}
		})
	}
}
