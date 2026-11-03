package stealth

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// proxyResolveTimeout bounds the lookup of a proxy host name when the proxy
// is configured. A failed lookup never fails SetProxy: the proxy simply gets
// no dial allowance (and a warning is logged), so a private address behind
// the name stays blocked; the next SetProxy with the same value retries.
const proxyResolveTimeout = 5 * time.Second

// maxRememberedProxies bounds how many distinct proxy URLs stay allowed on one
// client; the oldest is evicted first.
const maxRememberedProxies = 64

// lookupProxyHost resolves a proxy host name with the resolver net.Dialer
// uses. A variable so tests can fail the lookup without touching the network.
var lookupProxyHost = func(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// proxyEndpoint is the one network endpoint a configured proxy occupies: the
// addresses its host resolved to when the proxy was configured, and its port.
type proxyEndpoint struct {
	addrs []netip.Addr
	port  string
}

func (e proxyEndpoint) allows(ip netip.Addr, port string) bool {
	if port != e.port {
		return false
	}
	for _, a := range e.addrs {
		if a == ip {
			return true
		}
	}
	return false
}

// defaultProxyPort is the port a proxy URL without one is dialed on.
func defaultProxyPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	default: // socks4, socks5, socks5h
		return "1080"
	}
}

// resolveProxyEndpoint turns a configured proxy URL into the endpoint the
// dialer will connect to. It returns nil for "" (direct) and for anything
// unparsable, which leaves the guard fully closed. A host name is resolved
// with net.DefaultResolver — the resolver net.Dialer uses — and the result is
// pinned: a name that later rebinds to another address is not allowed on the
// strength of the name.
func resolveProxyEndpoint(raw string) *proxyEndpoint {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	port := u.Port()
	if port == "" {
		port = defaultProxyPort(u.Scheme)
	}
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		return &proxyEndpoint{addrs: []netip.Addr{ip.Unmap()}, port: port}
	}
	ctx, cancel := context.WithTimeout(context.Background(), proxyResolveTimeout)
	defer cancel()
	ips, err := lookupProxyHost(ctx, host)
	if err != nil || len(ips) == 0 {
		// Never log the URL: it can carry credentials.
		slog.Warn("stealth: proxy host lookup failed; dials to the proxy stay SSRF-guarded and a private proxy address will be refused until SetProxy is called again",
			slog.String("host", host), slog.Any("error", err))
		return nil
	}
	addrs := make([]netip.Addr, len(ips))
	for i, ip := range ips {
		addrs[i] = ip.WithZone("").Unmap()
	}
	return &proxyEndpoint{addrs: addrs, port: port}
}

// proxyAllowance is the set of proxy endpoints the client's dial guard lets
// through: every proxy configured on this client recently (the last
// maxRememberedProxies distinct URLs). All of them are operator configuration,
// not remote input, so the guard's job (stop a remote party steering a dial to
// an internal address) does not apply to them. Everything else stays guarded.
//
// A set rather than only the latest proxy, because under rotation a request
// that read proxy P1 may dial after another goroutine switched to P2; the
// guard cannot see which proxy a given dial was meant for.
type proxyAllowance struct {
	mu      sync.Mutex
	order   []string                  // distinct URLs, oldest first
	entries map[string]*proxyEndpoint // successfully resolved URLs only
	snap    atomic.Pointer[[]proxyEndpoint]
}

func newProxyAllowance(initial string) *proxyAllowance {
	a := &proxyAllowance{entries: make(map[string]*proxyEndpoint)}
	if ep := resolveProxyEndpoint(initial); ep != nil {
		a.remember(initial, ep)
	}
	return a
}

// remember records ep for url as the most recent proxy, evicts beyond the
// bound and republishes the lock-free snapshot. Caller holds mu (or is the
// constructor).
func (a *proxyAllowance) remember(url string, ep *proxyEndpoint) {
	for i, u := range a.order {
		if u == url {
			a.order = append(a.order[:i], a.order[i+1:]...)
			break
		}
	}
	a.order = append(a.order, url)
	a.entries[url] = ep
	for len(a.order) > maxRememberedProxies {
		delete(a.entries, a.order[0])
		a.order = a.order[1:]
	}
	a.publish()
}

func (a *proxyAllowance) forget(url string) {
	for i, u := range a.order {
		if u == url {
			a.order = append(a.order[:i], a.order[i+1:]...)
			break
		}
	}
	delete(a.entries, url)
	a.publish()
}

func (a *proxyAllowance) publish() {
	list := make([]proxyEndpoint, 0, len(a.order))
	for _, u := range a.order {
		list = append(list, *a.entries[u])
	}
	a.snap.Store(&list)
}

func (a *proxyAllowance) allows(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.WithZone("").Unmap()
	list := a.snap.Load()
	if list == nil {
		return false
	}
	for _, e := range *list {
		if e.allows(ip, port) {
			return true
		}
	}
	return false
}

// wrap returns a dial guard that passes the remembered proxy endpoints and
// defers to inner for every other address.
func (a *proxyAllowance) wrap(inner func(network, address string) error) func(network, address string) error {
	return func(network, address string) error {
		switch network {
		case "unix", "unixgram", "unixpacket":
		default:
			if a.allows(address) {
				return nil
			}
		}
		return inner(network, address)
	}
}

// apply switches the backend to proxyURL through set. The endpoint is allowed
// before set runs, so a dial the backend makes during the switch is not
// refused. A proxy that was not remembered before and that set rejects is
// forgotten again. The lookup runs outside the lock, and only when this URL
// has no successfully resolved endpoint yet (a failed lookup is retried).
func (a *proxyAllowance) apply(proxyURL string, set func(string) error) error {
	a.mu.Lock()
	ep, known := a.entries[proxyURL]
	a.mu.Unlock()
	if !known {
		ep = resolveProxyEndpoint(proxyURL)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	_, wasKnown := a.entries[proxyURL]
	if ep != nil {
		a.remember(proxyURL, ep)
	}
	if err := set(proxyURL); err != nil {
		if ep != nil && !wasKnown {
			a.forget(proxyURL)
		}
		return err
	}
	return nil
}
