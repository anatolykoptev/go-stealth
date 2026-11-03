package stealth

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tunnelProxy is a minimal HTTP CONNECT proxy on a loopback listener. It
// tunnels every CONNECT to a fixed upstream, whatever host the client asked
// for, so a test can name a public-looking target without any outside traffic.
type tunnelProxy struct {
	ln       net.Listener
	upstream string
	connects atomic.Int32

	mu    sync.Mutex
	conns []net.Conn
	wg    sync.WaitGroup
}

func startTunnelProxy(t *testing.T, upstream string) *tunnelProxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	p := &tunnelProxy{ln: ln, upstream: upstream}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			p.track(c)
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				p.serve(c)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		p.mu.Lock()
		for _, c := range p.conns {
			_ = c.Close()
		}
		p.mu.Unlock()
		p.wg.Wait()
	})
	return p
}

func (p *tunnelProxy) track(c net.Conn) {
	p.mu.Lock()
	p.conns = append(p.conns, c)
	p.mu.Unlock()
}

func (p *tunnelProxy) addr() string { return p.ln.Addr().String() }

func (p *tunnelProxy) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil || req.Method != http.MethodConnect {
		return
	}
	p.connects.Add(1)
	up, err := net.Dial("tcp", p.upstream)
	if err != nil {
		return
	}
	p.track(up)
	defer func() { _ = up.Close() }()
	if _, err := io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(up, br)
		close(done)
	}()
	_, _ = io.Copy(c, up)
	_ = up.Close()
	<-done
}

func startTLSUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "via-proxy-ok")
	}))
	t.Cleanup(srv.Close)
	return srv
}

// captureDialControl builds a client whose backend factory records the dial
// guard the client hands to its backend, so a test can drive the exact guard
// the real dialer runs. The std backend is built so the client stays usable.
func captureDialControl(t *testing.T, opts ...ClientOption) (*BrowserClient, func(addr string) error) {
	t.Helper()
	var ctl func(network, address string) error
	factory := func(cfg BackendConfig) (HTTPDoer, error) {
		ctl = cfg.DialControl
		return newStdBackend(cfg)
	}
	all := append([]ClientOption{WithBackend(factory)}, opts...)
	bc, err := NewClient(all...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if ctl == nil {
		t.Fatal("backend received no dial guard")
	}
	return bc, func(addr string) error { return ctl("tcp", addr) }
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// Test 1: a client whose operator-configured proxy sits on a private
// (loopback) address reaches a target through it, on both backends.
func TestSSRFProxy_PrivateProxyReachesTarget(t *testing.T) {
	up := startTLSUpstream(t)
	// The target is a public-looking literal that passes the default URL
	// guard; the proxy ignores it and tunnels to the local upstream.
	const target = "https://198.51.100.7/ok"

	backends := []struct {
		name string
		opts []ClientOption
	}{
		{"tls-client", nil},
		{"std", []ClientOption{WithStdHTTP()}},
	}
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			p := startTunnelProxy(t, up.Listener.Addr().String())
			opts := append([]ClientOption{
				WithProxy("http://" + p.addr()),
				WithInsecureSkipVerify(),
				WithTimeout(10),
			}, b.opts...)
			bc, err := NewClient(opts...)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			t.Cleanup(func() { _ = bc.Close() })

			body, _, status, err := bc.Do(http.MethodGet, target, nil, nil)
			if err != nil {
				t.Fatalf("request through private proxy failed: %v", err)
			}
			if status != http.StatusOK || string(body) != "via-proxy-ok" {
				t.Fatalf("status=%d body=%q, want 200 via-proxy-ok", status, body)
			}
			if p.connects.Load() == 0 {
				t.Fatal("request did not go through the proxy")
			}
		})
	}
}

// The destination check still runs on a proxied request: a private target is
// refused before the proxy is contacted, even though the proxy itself is
// private and allowed.
func TestSSRFProxy_DestinationStillChecked(t *testing.T) {
	up := startTLSUpstream(t)
	p := startTunnelProxy(t, up.Listener.Addr().String())
	bc, err := NewClient(WithProxy("http://"+p.addr()), WithInsecureSkipVerify(), WithTimeout(10))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = bc.Close() })

	for _, target := range []string{"https://" + up.Listener.Addr().String() + "/", "https://10.9.0.7/"} {
		_, _, _, err = bc.Do(http.MethodGet, target, nil, nil)
		if !errors.Is(err, ErrSSRFBlocked) {
			t.Fatalf("private destination %s not blocked: err=%v", target, err)
		}
	}
	if got := p.connects.Load(); got != 0 {
		t.Fatalf("proxy contacted %d times for a blocked destination", got)
	}
}

// Test 2: only the proxy endpoint is allowed. A different loopback port, the
// same port on another address, and any private address stay blocked — and a
// real dialer wearing the guard is refused before connect(2).
func TestSSRFProxy_OnlyProxyEndpointAllowed(t *testing.T) {
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = proxyLn.Close() })
	otherLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = otherLn.Close() })
	go acceptAndClose(proxyLn)
	go acceptAndClose(otherLn)

	proxyAddr := proxyLn.Addr().String()
	_, proxyPort, _ := net.SplitHostPort(proxyAddr)

	_, guard := captureDialControl(t, WithProxy("http://"+proxyAddr))

	if err := guard(proxyAddr); err != nil {
		t.Fatalf("configured proxy endpoint blocked: %v", err)
	}
	blocked := []string{
		otherLn.Addr().String(),  // other loopback port, i.e. a direct dial
		"127.0.0.2:" + proxyPort, // same port, another address
		"10.9.0.7:" + proxyPort,  // private, same port
		"169.254.169.254:80",     // metadata
		"[::1]:" + proxyPort,     // IPv6 loopback, same port
	}
	for _, a := range blocked {
		if err := guard(a); !errors.Is(err, ErrSSRFBlocked) {
			t.Errorf("guard(%s) = %v, want ErrSSRFBlocked", a, err)
		}
	}

	d := net.Dialer{Control: adaptControl(func(n, a string) error { return guard(a) }), Timeout: 2 * time.Second}
	if c, err := d.DialContext(context.Background(), "tcp", otherLn.Addr().String()); !errors.Is(err, ErrSSRFBlocked) {
		if c != nil {
			_ = c.Close()
		}
		t.Fatalf("direct dial to another loopback port: err=%v, want ErrSSRFBlocked", err)
	}
	c, err := d.DialContext(context.Background(), "tcp", proxyAddr)
	if err != nil {
		t.Fatalf("dial to the proxy endpoint refused: %v", err)
	}
	_ = c.Close()
}

// A client with no proxy gets no allowance at all.
func TestSSRFProxy_NoProxyNoAllowance(t *testing.T) {
	_, guard := captureDialControl(t)
	if err := guard("127.0.0.1:" + strconv.Itoa(8080)); !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("no proxy configured but loopback allowed: %v", err)
	}
}

// A proxy given by name is allowed only at the addresses the name resolved to
// when it was configured, on its port.
func TestSSRFProxy_NamedProxyPinnedToResolvedAddresses(t *testing.T) {
	_, guard := captureDialControl(t, WithProxy("http://localhost:3128"))

	if err := guard("127.0.0.1:3128"); err != nil {
		t.Fatalf("resolved address of the proxy name blocked: %v", err)
	}
	for _, a := range []string{"127.0.0.2:3128", "127.0.0.1:3129", "10.0.0.1:3128"} {
		if err := guard(a); !errors.Is(err, ErrSSRFBlocked) {
			t.Errorf("guard(%s) = %v, want ErrSSRFBlocked (name must not allow other addresses)", a, err)
		}
	}
}

// SetProxy adds the new proxy to the allowed set: it works end to end, the
// previous proxy stays reachable for a request still in flight, and an address
// that was never a proxy stays blocked.
func TestSSRFProxy_SetProxyAllowsNewProxy(t *testing.T) {
	up := startTLSUpstream(t)
	p1 := startTunnelProxy(t, up.Listener.Addr().String())
	p2 := startTunnelProxy(t, up.Listener.Addr().String())
	never := freeLoopbackAddr(t)

	var guard func(network, address string) error
	factory := func(cfg BackendConfig) (HTTPDoer, error) {
		guard = cfg.DialControl
		return newTLSClientBackend(cfg)
	}
	bc, err := NewClient(WithBackend(factory), WithProxy("http://"+p1.addr()),
		WithInsecureSkipVerify(), WithTimeout(10))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = bc.Close() })

	if err := guard("tcp", p2.addr()); !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("precondition: second proxy allowed before SetProxy: %v", err)
	}
	if err := bc.SetProxy("http://" + p2.addr()); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	if err := guard("tcp", p2.addr()); err != nil {
		t.Fatalf("new proxy blocked after SetProxy: %v", err)
	}
	if err := guard("tcp", p1.addr()); err != nil {
		t.Fatalf("recent proxy refused after rotation (in-flight dial would fail): %v", err)
	}
	if err := guard("tcp", never); !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("address that was never a proxy allowed: %v", err)
	}

	body, _, status, err := bc.Do(http.MethodGet, "https://198.51.100.7/ok", nil, nil)
	if err != nil || status != http.StatusOK || string(body) != "via-proxy-ok" {
		t.Fatalf("request via new proxy: status=%d body=%q err=%v", status, body, err)
	}
	if p2.connects.Load() == 0 || p1.connects.Load() != 0 {
		t.Fatalf("request not served by the new proxy: p1=%d p2=%d", p1.connects.Load(), p2.connects.Load())
	}
}

// Only the most recent 64 distinct proxies stay allowed; an
// evicted one is blocked again.
func TestSSRFProxy_OldestProxyEvicted(t *testing.T) {
	first := "127.0.0.1:20000"
	bc, guard := captureDialControl(t, WithProxy("http://"+first))
	for i := 1; i <= maxRememberedProxies; i++ {
		if err := bc.SetProxy("http://127.0.0.1:" + strconv.Itoa(20000+i)); err != nil {
			t.Fatalf("SetProxy: %v", err)
		}
	}
	if err := guard(first); !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("evicted proxy still allowed: %v", err)
	}
	if err := guard("127.0.0.1:" + strconv.Itoa(20000+1)); err != nil {
		t.Fatalf("recent proxy evicted too early: %v", err)
	}
	if err := guard("127.0.0.1:" + strconv.Itoa(20000+maxRememberedProxies)); err != nil {
		t.Fatalf("newest proxy blocked: %v", err)
	}
}

// A target URL naming the proxy's own address and port is refused by the
// request-URL guard, and the proxy sees no CONNECT.
func TestSSRFProxy_TargetNamingProxyRefused(t *testing.T) {
	up := startTLSUpstream(t)
	p := startTunnelProxy(t, up.Listener.Addr().String())
	bc, err := NewClient(WithProxy("http://"+p.addr()), WithInsecureSkipVerify(), WithTimeout(10))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = bc.Close() })

	_, _, _, err = bc.Do(http.MethodGet, "https://"+p.addr()+"/", nil, nil)
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("target naming the proxy's own endpoint not refused: err=%v", err)
	}
	if got := p.connects.Load(); got != 0 {
		t.Fatalf("proxy saw %d CONNECT for a refused target", got)
	}
}

// failingProxyDoer fails every SetProxy, as a backend that cannot apply it.
type failingProxyDoer struct{ HTTPDoer }

func (failingProxyDoer) SetProxy(string) error { return errors.New("cannot apply proxy") }

// A SetProxy the backend rejects leaves the previous proxy (and so its
// allowance) in force, and does not open the new address.
func TestSSRFProxy_FailedSetProxyKeepsOldAllowance(t *testing.T) {
	var guard func(network, address string) error
	factory := func(cfg BackendConfig) (HTTPDoer, error) {
		guard = cfg.DialControl
		d, err := newStdBackend(cfg)
		return failingProxyDoer{d}, err
	}
	p1, p2 := freeLoopbackAddr(t), freeLoopbackAddr(t)
	bc, err := NewClient(WithBackend(factory), WithProxy("http://"+p1))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := bc.SetProxy("http://" + p2); err == nil {
		t.Fatal("SetProxy: want error")
	}
	if err := guard("tcp", p1); err != nil {
		t.Fatalf("old proxy lost its allowance after a failed SetProxy: %v", err)
	}
	if err := guard("tcp", p2); !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("rejected proxy was allowed: %v", err)
	}
}

func acceptAndClose(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		_ = c.Close()
	}
}

type logBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *logBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// A failed proxy lookup logs a warning naming the host (never the userinfo),
// and a later SetProxy with the same value retries the lookup instead of
// leaving the proxy dead.
func TestSSRFProxy_FailedLookupWarnsAndRetries(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	var lookups atomic.Int32
	orig := lookupProxyHost
	lookupProxyHost = func(_ context.Context, host string) ([]netip.Addr, error) {
		lookups.Add(1)
		if fail.Load() {
			return nil, errors.New("lookup " + host + ": no such host")
		}
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}
	buf := &logBuf{}
	origLog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, nil)))
	t.Cleanup(func() {
		lookupProxyHost = orig
		slog.SetDefault(origLog)
	})

	const proxy = "http://operator:s3cr3t@wg-proxy.test:3128"
	bc, guard := captureDialControl(t, WithProxy(proxy))

	out := buf.String()
	if !strings.Contains(out, "wg-proxy.test") {
		t.Fatalf("no warning naming the proxy host after a failed lookup; log=%q", out)
	}
	if strings.Contains(out, "s3cr3t") || strings.Contains(out, "operator") {
		t.Fatalf("warning leaks proxy credentials: %q", out)
	}
	if err := guard("127.0.0.1:3128"); !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("unresolved proxy allowed: %v", err)
	}

	fail.Store(false)
	if err := bc.SetProxy(proxy); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	if err := guard("127.0.0.1:3128"); err != nil {
		t.Fatalf("same-value SetProxy did not retry the failed lookup: %v", err)
	}

	n := lookups.Load()
	if err := bc.SetProxy(proxy); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	if got := lookups.Load(); got != n {
		t.Fatalf("lookup repeated for a value that already resolved (%d -> %d)", n, got)
	}
}
