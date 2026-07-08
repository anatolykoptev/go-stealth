package stealth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

// backendCase parametrizes the SSRF tests over both production backends. The
// tls-client row is load-bearing: the entire direct-path rebind defense rests
// on tls-client's WithDialer(Control) firing at connect — a std-only test
// would pass while the production backend silently regressed.
type backendCase struct {
	name string
	opt  ClientOption
}

func backends() []backendCase {
	return []backendCase{
		{"std", WithStdHTTP()},
		{"tls", WithBackend(newTLSClientBackend)},
	}
}

func mustPort(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	return u.Port()
}

// --- tier 2: per-hop redirect guard blocks an internal redirect target ------

func TestSSRFRedirectGuard_BlocksInternalHop(t *testing.T) {
	for _, be := range backends() {
		t.Run(be.name, func(t *testing.T) {
			var internalHits atomic.Int32
			internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				internalHits.Add(1)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("SECRET"))
			}))
			defer internal.Close()
			internalPort := mustPort(t, internal.URL)

			redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", internal.URL+"/latest/meta-data")
				w.WriteHeader(http.StatusFound)
			}))
			defer redirector.Close()

			// A guard that blocks ONLY the "internal" target's port, so the
			// loopback test infra (the redirector) is reachable but the
			// redirect hop to the internal target is refused per-hop.
			blockInternal := func(req *http.Request, _ []*http.Request) error {
				if req.URL.Port() == internalPort {
					return errBlockedTest
				}
				return nil
			}

			// Guarded: redirect to internal is blocked, 0 bytes egress.
			guarded, err := NewClient(be.opt, WithFollowRedirects(), WithoutSSRFGuard(), WithRedirectGuard(blockInternal))
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, _, _, err = guarded.Do(http.MethodGet, redirector.URL, nil, nil)
			if err == nil {
				t.Fatalf("expected redirect-to-internal to be blocked, got nil error")
			}
			if got := internalHits.Load(); got != 0 {
				t.Fatalf("internal target hit %d times through the guard; want 0", got)
			}

			// Positive control (proves the test is non-vacuous / RED-on-revert):
			// with the guard removed the SAME redirect IS followed to the
			// internal target.
			internalHits.Store(0)
			unguarded, err := NewClient(be.opt, WithFollowRedirects(), WithoutSSRFGuard())
			if err != nil {
				t.Fatalf("NewClient(control): %v", err)
			}
			body, _, _, err := unguarded.Do(http.MethodGet, redirector.URL, nil, nil)
			if err != nil {
				t.Fatalf("control fetch failed: %v", err)
			}
			if internalHits.Load() == 0 {
				t.Fatalf("control: redirect was NOT followed — test is vacuous")
			}
			if string(body) != "SECRET" {
				t.Fatalf("control: want SECRET body, got %q", body)
			}
		})
	}
}

// --- tier 1: connect-time dial control blocks an internal redirect target ---

func TestSSRFDialControl_BlocksInternalHop(t *testing.T) {
	for _, be := range backends() {
		t.Run(be.name, func(t *testing.T) {
			var internalHits atomic.Int32
			internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				internalHits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer internal.Close()
			internalPort := mustPort(t, internal.URL)

			redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", internal.URL+"/pwn")
				w.WriteHeader(http.StatusFound)
			}))
			defer redirector.Close()

			// Connect-time guard blocking only the internal port — proves the
			// dial hook fires on each redirect hop's dial on the real backend.
			blockDial := func(_, address string) error {
				if _, port, _ := net.SplitHostPort(address); port == internalPort {
					return errBlockedTest
				}
				return nil
			}

			guarded, err := NewClient(be.opt, WithFollowRedirects(), WithoutSSRFGuard(), WithDialControl(blockDial))
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, _, _, err = guarded.Do(http.MethodGet, redirector.URL, nil, nil)
			if err == nil {
				t.Fatalf("expected dial to internal to be refused, got nil error")
			}
			if got := internalHits.Load(); got != 0 {
				t.Fatalf("internal target dialed %d times through the guard; want 0", got)
			}

			// Positive control: without the dial guard, the hop is dialed.
			internalHits.Store(0)
			unguarded, err := NewClient(be.opt, WithFollowRedirects(), WithoutSSRFGuard())
			if err != nil {
				t.Fatalf("NewClient(control): %v", err)
			}
			_, _, _, _ = unguarded.Do(http.MethodGet, redirector.URL, nil, nil)
			if internalHits.Load() == 0 {
				t.Fatalf("control: internal target was never dialed — test is vacuous")
			}
		})
	}
}

// --- fail-closed by construction: zero-option client refuses loopback -------

func TestSSRFFailClosedByDefault(t *testing.T) {
	for _, be := range backends() {
		t.Run(be.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("ok"))
			}))
			defer srv.Close()

			// Zero SSRF options → fail-closed: the built-in default-deny
			// refuses the loopback target at the pre-request tier.
			guarded, err := NewClient(be.opt)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, _, _, err = guarded.Do(http.MethodGet, srv.URL, nil, nil)
			if !errors.Is(err, ErrSSRFBlocked) {
				t.Fatalf("zero-option client did not fail closed: err=%v", err)
			}

			// WithoutSSRFGuard opts out (tests only) so loopback works.
			open, err := NewClient(be.opt, WithoutSSRFGuard())
			if err != nil {
				t.Fatalf("NewClient(opt-out): %v", err)
			}
			body, _, status, err := open.Do(http.MethodGet, srv.URL, nil, nil)
			if err != nil {
				t.Fatalf("opt-out fetch failed: %v", err)
			}
			if status != http.StatusOK || string(body) != "ok" {
				t.Fatalf("opt-out fetch: status=%d body=%q", status, body)
			}
		})
	}
}

// --- tier 3: pre-request guard covers a PROXIED fetch -----------------------

// A proxied client's dial control (tier 1) sees only the proxy address, so it
// cannot catch an internal target. The pre-request guard (tier 3) is the only
// tier that does: it must refuse the internal target BEFORE the proxy is even
// contacted.
func TestSSRFPreRequestGuard_ProxiedTargetBlockedBeforeProxyDial(t *testing.T) {
	var proxyConns atomic.Int32
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			proxyConns.Add(1)
			_ = c.Close()
		}
	}()

	// Default guards on; a proxy is configured. The initial target is an
	// internal (private) literal.
	client, err := NewClient(WithStdHTTP(), WithProxy("http://"+ln.Addr().String()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, _, _, err = client.Do(http.MethodGet, "http://10.0.0.5/latest/meta-data", nil, nil)
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("proxied internal fetch not blocked pre-request: err=%v", err)
	}
	if got := proxyConns.Load(); got != 0 {
		t.Fatalf("proxy was contacted %d times; tier-3 must block before any proxy dial", got)
	}
}

// --- SSRF block is not retried under proxy rotation --------------------------

// An SSRF-blocked verdict is about the target URL/address, not about which
// proxy served the request — every retry would re-block identically (the
// pre-request guard runs before SetProxy on the next attempt too). doWithRetry
// must return immediately on ErrSSRFBlocked instead of burning the proxy pool.
func TestSSRFBlocked_NotRetriedUnderProxyRotation(t *testing.T) {
	pool := &mockPool{proxies: []string{"p1", "p2", "p3"}}
	client, err := NewClient(WithStdHTTP(), WithProxyPool(pool), WithRetryOnBlock(2))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, _, _, err = client.Do(http.MethodGet, "http://10.8.0.1:8914/internal", nil, nil)
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("expected ErrSSRFBlocked, got %v", err)
	}
	// blockRetries=2 would allow up to 3 attempts (3 pool.Next() calls) on an
	// ordinary error; an SSRF block must stop after the first.
	if got := pool.idx.Load(); got != 1 {
		t.Fatalf("proxy pool Next() called %d times for a blocked target; want 1 (no retry)", got)
	}
}

// --- default-deny unit coverage ---------------------------------------------

func TestDefaultDenyDial(t *testing.T) {
	tests := []struct {
		addr    string
		blocked bool
	}{
		{"169.254.169.254:80", true}, // cloud metadata (link-local)
		{"10.8.0.1:8914", true},      // private (WireGuard go-grad)
		{"127.0.0.1:8080", true},     // loopback
		{"192.168.1.1:80", true},     // private
		{"0.0.0.0:80", true},         // unspecified
		{"[::1]:80", true},           // IPv6 loopback
		{"93.184.216.34:443", false}, // public (example.com)
		{"1.1.1.1:53", false},        // public
	}
	for _, tt := range tests {
		err := defaultDenyDial("tcp", tt.addr)
		if tt.blocked && !errors.Is(err, ErrSSRFBlocked) {
			t.Errorf("defaultDenyDial(%q): want blocked, got %v", tt.addr, err)
		}
		if !tt.blocked && err != nil {
			t.Errorf("defaultDenyDial(%q): want allowed, got %v", tt.addr, err)
		}
	}
	// Unix sockets are a local path, not an SSRF vector.
	if err := defaultDenyDial("unix", "/var/run/x.sock"); err != nil {
		t.Errorf("unix socket should be allowed: %v", err)
	}
}

func TestDefaultDenyURL(t *testing.T) {
	tests := []struct {
		raw     string
		blocked bool
	}{
		{"http://169.254.169.254/latest/meta-data", true},
		{"http://10.8.0.1:8914/ads", true},
		{"http://127.0.0.1:9000/", true},
		{"https://[::1]/", true},
		{"file:///etc/passwd", true},            // scheme not allowed
		{"gopher://127.0.0.1:6379/_INFO", true}, // scheme not allowed
		{"https://93.184.216.34/", false},       // public literal
	}
	for _, tt := range tests {
		u, err := url.Parse(tt.raw)
		if err != nil {
			t.Fatalf("parse %q: %v", tt.raw, err)
		}
		err = defaultDenyURL(context.Background(), u)
		if tt.blocked && !errors.Is(err, ErrSSRFBlocked) {
			t.Errorf("defaultDenyURL(%q): want blocked, got %v", tt.raw, err)
		}
		if !tt.blocked && err != nil {
			t.Errorf("defaultDenyURL(%q): want allowed, got %v", tt.raw, err)
		}
	}
}

// --- redirect hop cap (footgun guard) ---------------------------------------

func TestDefaultDenyRedirect_HopCap(t *testing.T) {
	req := &http.Request{URL: mustURL(t, "https://example.com/")}
	// Below the cap with a public URL → allowed.
	if err := defaultDenyRedirect(req, make([]*http.Request, maxSSRFRedirectHops-1)); err != nil {
		t.Fatalf("hop %d should be allowed: %v", maxSSRFRedirectHops-1, err)
	}
	// At the cap → refused (proves the closure re-owns the cap that overriding
	// CheckRedirect drops).
	if err := defaultDenyRedirect(req, make([]*http.Request, maxSSRFRedirectHops)); err == nil {
		t.Fatalf("hop %d should be refused by the cap", maxSSRFRedirectHops)
	}
}

func TestRedirectLoop_Terminates(t *testing.T) {
	var hits atomic.Int32
	var self *httptest.Server
	self = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Location", self.URL)
		w.WriteHeader(http.StatusFound)
	}))
	defer self.Close()

	// A redirect guard that owns ONLY the cap (allows loopback) — proves a
	// custom redirect hook without a cap would loop until timeout.
	capOnly := func(_ *http.Request, via []*http.Request) error {
		if len(via) >= maxSSRFRedirectHops {
			return errBlockedTest
		}
		return nil
	}
	client, err := NewClient(WithStdHTTP(), WithFollowRedirects(), WithoutSSRFGuard(), WithRedirectGuard(capOnly))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, _, _, err := client.Do(http.MethodGet, self.URL, nil, nil); err == nil {
		t.Fatalf("self-redirect should terminate with an error, not hang")
	}
	if got := hits.Load(); got > maxSSRFRedirectHops+1 {
		t.Fatalf("self-redirect followed %d times; cap is %d", got, maxSSRFRedirectHops)
	}
}

// --- fhttp -> stdlib redirect adapter (tls-only wiring) ----------------------

func TestAdaptRedirect(t *testing.T) {
	var gotURL *url.URL
	var gotVia int
	guard := func(req *http.Request, via []*http.Request) error {
		gotURL = req.URL
		gotVia = len(via)
		return errBlockedTest
	}
	adapted := adaptRedirect(guard)

	target := mustURL(t, "http://10.8.0.1:8914/internal")
	fr := &fhttp.Request{Method: http.MethodGet, URL: target}
	fvia := make([]*fhttp.Request, 3)

	err := adapted(fr, fvia)
	if !errors.Is(err, errBlockedTest) {
		t.Fatalf("adapter did not propagate the guard error: %v", err)
	}
	if gotURL == nil || gotURL.String() != target.String() {
		t.Fatalf("adapter fed wrong URL to the guard: %v", gotURL)
	}
	if gotVia != len(fvia) {
		t.Fatalf("adapter fed hop count %d; want %d", gotVia, len(fvia))
	}
}

// TestAdaptRedirect_FaithfulVia proves the via chain the tls backend feeds a
// PUBLIC WithRedirectGuard is populated hop-by-hop (URL/Method/Header), not a
// length-only slice of empty requests — a caller-supplied guard that
// dereferences via[i].URL or via[i].Header must not nil-panic or read
// garbage.
func TestAdaptRedirect_FaithfulVia(t *testing.T) {
	var gotVia []*http.Request
	guard := func(_ *http.Request, via []*http.Request) error {
		gotVia = via
		return nil
	}
	adapted := adaptRedirect(guard)

	hop0 := &fhttp.Request{Method: http.MethodGet, URL: mustURL(t, "https://a.example/start"), Header: fhttp.Header{"X-Hop": []string{"0"}}}
	hop1 := &fhttp.Request{Method: http.MethodGet, URL: mustURL(t, "https://b.example/mid"), Header: fhttp.Header{"X-Hop": []string{"1"}}}
	fr := &fhttp.Request{Method: http.MethodGet, URL: mustURL(t, "https://c.example/final")}
	fvia := []*fhttp.Request{hop0, hop1}

	if err := adapted(fr, fvia); err != nil {
		t.Fatalf("adapted: %v", err)
	}
	if len(gotVia) != 2 {
		t.Fatalf("via length = %d, want 2", len(gotVia))
	}
	if gotVia[0] == nil || gotVia[0].URL == nil || gotVia[0].URL.String() != "https://a.example/start" {
		t.Fatalf("via[0] not faithfully populated: %+v", gotVia[0])
	}
	if gotVia[0].Header.Get("X-Hop") != "0" {
		t.Fatalf("via[0].Header not populated: %v", gotVia[0].Header)
	}
	if gotVia[1] == nil || gotVia[1].URL == nil || gotVia[1].URL.String() != "https://b.example/mid" {
		t.Fatalf("via[1] not faithfully populated: %+v", gotVia[1])
	}
	if gotVia[1].Header.Get("X-Hop") != "1" {
		t.Fatalf("via[1].Header not populated: %v", gotVia[1].Header)
	}
}

// TestAdaptRedirect_NilHopSafe proves a nil entry in the fhttp via chain
// (defensive — fhttp's own loop never produces one, but a public adapter must
// not assume that) still yields a non-nil *http.Request, so a guard that
// unconditionally dereferences via[i] cannot nil-panic.
func TestAdaptRedirect_NilHopSafe(t *testing.T) {
	guard := func(_ *http.Request, via []*http.Request) error {
		for i, v := range via {
			if v == nil {
				t.Fatalf("via[%d] is nil", i)
			}
			_ = v.URL // must not panic even though URL is nil for this hop
		}
		return nil
	}
	adapted := adaptRedirect(guard)
	fr := &fhttp.Request{Method: http.MethodGet, URL: mustURL(t, "https://example.com/")}
	if err := adapted(fr, []*fhttp.Request{nil}); err != nil {
		t.Fatalf("adapted: %v", err)
	}
}

// --- fidelity: permissive guards do not break a legitimate redirect ---------

func TestSSRFGuards_LegitRedirectStillFollowed(t *testing.T) {
	for _, be := range backends() {
		t.Run(be.name, func(t *testing.T) {
			var finalHits atomic.Int32
			var gotReferer string
			final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				finalHits.Add(1)
				gotReferer = r.Header.Get("Referer")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("final"))
			}))
			defer final.Close()

			start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", final.URL+"/dest")
				w.WriteHeader(http.StatusMovedPermanently)
			}))
			defer start.Close()

			allowDial := func(_, _ string) error { return nil }
			allowRedirect := func(_ *http.Request, _ []*http.Request) error { return nil }
			allowURL := func(_ context.Context, _ *url.URL) error { return nil }

			// Guards installed but permissive: the redirect must still be
			// followed through the guarded loop.
			client, err := NewClient(be.opt, WithFollowRedirects(),
				WithDialControl(allowDial), WithRedirectGuard(allowRedirect), WithRequestURLGuard(allowURL))
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			body, _, status, err := client.Do(http.MethodGet, start.URL, nil, nil)
			if err != nil {
				t.Fatalf("guarded legit redirect failed: %v", err)
			}
			if status != http.StatusOK || string(body) != "final" {
				t.Fatalf("redirect not followed: status=%d body=%q", status, body)
			}
			if finalHits.Load() == 0 {
				t.Fatalf("final destination never reached through the guarded loop")
			}
			_ = gotReferer // referer propagation is backend-dependent; presence not asserted
		})
	}
}

// errBlockedTest is a sentinel used by test guards.
var errBlockedTest = errors.New("blocked-by-test")

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
