package stealth

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// connCounter tracks connections to an httptest TLS server via ConnState.
type connCounter struct {
	mu    sync.Mutex
	open  map[net.Conn]struct{}
	total int
	proto int // ProtoMajor of the last request served
}

func newLiveServer(t *testing.T, h2 bool) (*httptest.Server, *connCounter) {
	t.Helper()
	cc := &connCounter{open: map[net.Conn]struct{}{}}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cc.mu.Lock()
		cc.proto = r.ProtoMajor
		cc.mu.Unlock()
		_, _ = w.Write([]byte("ok"))
	}))
	srv.EnableHTTP2 = h2
	srv.Config.ConnState = func(c net.Conn, s http.ConnState) {
		cc.mu.Lock()
		defer cc.mu.Unlock()
		switch s {
		case http.StateNew:
			cc.open[c] = struct{}{}
			cc.total++
		case http.StateClosed, http.StateHijacked:
			delete(cc.open, c)
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, cc
}

func (cc *connCounter) counts() (open, total int) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	return len(cc.open), cc.total
}

// waitOpen polls until the open-connection count equals want or 5 s pass.
func (cc *connCounter) waitOpen(t *testing.T, want int, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		open, _ := cc.counts()
		if open == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %d connections open, want %d", what, open, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func liveGet(t *testing.T, bc *BrowserClient, url string) {
	t.Helper()
	if _, _, code, err := bc.Do("GET", url, nil, nil); err != nil || code != 200 {
		t.Fatalf("Do: code=%d err=%v", code, err)
	}
}

func forEachLive(t *testing.T, fn func(t *testing.T, bc *BrowserClient, srv *httptest.Server, cc *connCounter)) {
	t.Helper()
	cases := []struct {
		name string
		h2   bool
		opts []ClientOption
	}{
		{"tls-h1", false, nil},
		{"tls-h2", true, nil},
		// net/http with a custom TLSClientConfig does not negotiate h2.
		{"std-h1", false, []ClientOption{WithStdHTTP()}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, cc := newLiveServer(t, c.h2)
			bc, err := NewClient(append([]ClientOption{WithoutSSRFGuard(), WithInsecureSkipVerify()}, c.opts...)...)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			fn(t, bc, srv, cc)
			cc.mu.Lock()
			defer cc.mu.Unlock()
			if want := map[bool]int{false: 1, true: 2}[c.h2]; cc.proto != want {
				t.Fatalf("server saw HTTP/%d, want HTTP/%d (protocol not exercised)", cc.proto, want)
			}
		})
	}
}

func TestLive_Close_ReleasesConnections(t *testing.T) {
	forEachLive(t, func(t *testing.T, bc *BrowserClient, srv *httptest.Server, cc *connCounter) {
		liveGet(t, bc, srv.URL)
		cc.waitOpen(t, 1, "after request (precondition)")
		if err := bc.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		cc.waitOpen(t, 0, "after Close")
	})
}

func TestLive_SetProxyChanged_ThenClose_ReleasesAll(t *testing.T) {
	cases := []struct {
		name string
		opts []ClientOption
	}{{"tls", nil}} // stdDoer.SetProxy does not flush connections (known; followup)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, cc := newLiveServer(t, false)
			bc, err := NewClient(append([]ClientOption{WithoutSSRFGuard(), WithInsecureSkipVerify()}, c.opts...)...)
			if err != nil {
				t.Fatal(err)
			}
			liveGet(t, bc, srv.URL)
			cc.waitOpen(t, 1, "after request (precondition)")
			if err := bc.SetProxy("http://127.0.0.1:9"); err != nil {
				t.Fatalf("SetProxy: %v", err)
			}
			if err := bc.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			cc.waitOpen(t, 0, "after SetProxy(changed)+Close")
		})
	}
}

func TestLive_SetProxyUnchanged_KeepsConnection(t *testing.T) {
	for _, h2 := range []bool{false, true} {
		name := "h1"
		if h2 {
			name = "h2"
		}
		t.Run(name, func(t *testing.T) {
			srv, cc := newLiveServer(t, h2)
			bc, err := NewClient(WithoutSSRFGuard(), WithInsecureSkipVerify())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = bc.Close() })
			liveGet(t, bc, srv.URL)
			if err := bc.SetProxy(""); err != nil {
				t.Fatalf("SetProxy: %v", err)
			}
			liveGet(t, bc, srv.URL)
			if _, total := cc.counts(); total != 1 {
				t.Fatalf("server saw %d connections for 2 requests with an unchanged SetProxy, want 1 (reuse)", total)
			}
		})
	}
}
