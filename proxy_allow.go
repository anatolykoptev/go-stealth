package stealth

import (
	"context"
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
// no dial allowance, so a private address behind the name stays blocked.
const proxyResolveTimeout = 5 * time.Second

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
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil
	}
	addrs := make([]netip.Addr, len(ips))
	for i, ip := range ips {
		addrs[i] = ip.WithZone("").Unmap()
	}
	return &proxyEndpoint{addrs: addrs, port: port}
}

// proxyAllowance tracks which proxy endpoint the client's backend is using and
// lets the dial guard pass exactly that endpoint. The proxy is operator
// configuration, not remote input, so the guard's job (stop a remote party
// steering a dial to an internal address) does not apply to it. Everything
// else the guard blocks stays blocked.
type proxyAllowance struct {
	mu  sync.Mutex // serialises apply
	eps atomic.Pointer[[]proxyEndpoint]
}

func newProxyAllowance(initial string) *proxyAllowance {
	a := &proxyAllowance{}
	a.store(resolveProxyEndpoint(initial))
	return a
}

func (a *proxyAllowance) store(eps ...*proxyEndpoint) {
	list := make([]proxyEndpoint, 0, len(eps))
	for _, e := range eps {
		if e != nil {
			list = append(list, *e)
		}
	}
	a.eps.Store(&list)
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
	list := a.eps.Load()
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

// wrap returns a dial guard that passes the current proxy endpoint and defers
// to inner for every other address.
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

// apply switches the backend to proxyURL through set and moves the allowance
// with it. While set runs both endpoints are allowed, so a dial the backend
// makes during the switch is not refused; afterwards only the proxy the
// backend actually ended up on is. A failed set leaves the old proxy (and
// allowance) in force.
func (a *proxyAllowance) apply(proxyURL string, set func(string) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.snapshot()
	next := resolveProxyEndpoint(proxyURL)
	a.store(append(old, next)...)
	if err := set(proxyURL); err != nil {
		a.store(old...)
		return err
	}
	a.store(next)
	return nil
}

func (a *proxyAllowance) snapshot() []*proxyEndpoint {
	list := a.eps.Load()
	if list == nil {
		return nil
	}
	out := make([]*proxyEndpoint, len(*list))
	for i := range *list {
		out[i] = &(*list)[i]
	}
	return out
}
