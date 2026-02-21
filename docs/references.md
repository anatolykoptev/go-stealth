# GitHub References & Patterns

Libraries and repos relevant to go-stealth development — organized by what to learn from each.

---

## TLS Fingerprint Impersonation

### enetx/surf ★1,325
https://github.com/enetx/surf

**Status**: Very active (Feb 2026), 6 months old, fastest growing.
**Stack**: Custom forks (enetx/utls, enetx/http, enetx/http2, enetx/http3). No bogdanfinn dependency.

**Steal these patterns**:
- **Fluent builder API**: `surf.NewClient().Builder().Impersonate().Chrome().Build()` — more ergonomic than functional options for complex configs
- **`Std()` method**: Converts to `net/http.Client` for standard library interop
- **Priority-ordered middleware**: `client.Use(middleware, priority)` — clean extensibility
- **Per-OS impersonation**: Chrome-Windows, Chrome-macOS, Chrome-Linux, Chrome-Android, Chrome-iOS as separate profiles
- **`RandomOS()` selection**: Random platform fingerprint per request
- **`sync.Pool` for decompressors**: Pool gzip/brotli/zstd readers to reduce GC pressure
- **DNS-over-TLS**: Built-in DoT resolver

**Files to study**:
- Builder pattern: `client_builder.go`
- Middleware system: `middleware.go`
- Browser profiles: `impersonate/` directory
- Std() conversion: `client.go` → `Std()` method

---

### sardanioss/httpcloak ★750
https://github.com/sardanioss/httpcloak

**Status**: Active (Feb 2026), 2 months old, fast growth.
**Stack**: Custom forks (sardanioss/utls, sardanioss/net, sardanioss/quic-go).

**Steal these patterns**:
- **Session abstraction**: `Session{ID, CreatedAt, LastUsed, RequestCount}` — stateful scraping with request counting
- **Distributed TLS session cache**: Backend interface for session resumption across instances
- **Connection pool health**: `IsHealthy()`, `Age()`, `IdleTime()` per connection
- **Platform auto-detection**: `runtime.GOOS` → match UA to actual platform
- **Client Hints auto-detection**: Parse server `Accept-CH` header, auto-generate matching `sec-ch-ua-*`
- **ECH (Encrypted Client Hello)**: Hides SNI from network observers
- **Clean package separation**: `fingerprint/`, `session/`, `pool/`, `transport/`, `proxy/`, `dns/`

**Files to study**:
- Session management: `session/`
- Connection health: `pool/`
- Fingerprint profiles: `fingerprint/`
- ECH implementation: `transport/`

---

### bogdanfinn/tls-client ★1,466
https://github.com/bogdanfinn/tls-client

**Status**: Active (Feb 2026). Our direct dependency.
**Stack**: Forked utls + fhttp.

**What we already use**: TLS fingerprint profiles, header ordering, cookie jar, proxy.

**New features to leverage** (since our initial integration):
- **Pre/post request hooks** (Feb 2026) — middleware-like extensibility at the transport level
- **Protocol racing** — Chrome-like H2/H3 "Happy Eyeballs" negotiation
- **WebSocket support** — fingerprinted WebSocket connections
- **Bandwidth tracking** — request/response size monitoring

**Files to study**:
- Request hooks: `hooks.go`
- Protocol negotiation: `client.go` → protocol racing logic
- Profile list: `profiles/` — latest browser versions

---

### Noooste/azuretls-client ★426
https://github.com/Noooste/azuretls-client

**Status**: Moderate (Jan 2026).
**Stack**: Own forks (Noooste/fhttp, Noooste/utls, Noooste/uquic-go).

**Steal these patterns**:
- **Proxy chaining**: Multi-hop proxies (proxy → proxy → target) for enhanced anonymity
- **Zero-config defaults**: Chrome fingerprint enabled by default, no setup required
- **Request dump/debug**: `session.Dump()` for request/response inspection
- **Edge browser preset**: Microsoft Edge profiles (go-stealth lacks this)

**Files to study**:
- Proxy chaining: `proxy.go`
- Session API: `session.go`
- Debug mode: `dump.go`

---

### juzeon/spoofed-round-tripper ★52
https://github.com/juzeon/spoofed-round-tripper

**Status**: Abandoned (Dec 2023), but the pattern is valuable.

**Key pattern**: Wraps tls-client as standard `http.RoundTripper`:
```go
transport := spoofedrt.NewRoundTripper(tls_client.Chrome_131)
client := &http.Client{Transport: transport}
// Now works with resty, go-retryablehttp, or any net/http consumer
```

**Directly applicable**: go-stealth should expose `RoundTripper` interface.

---

## Scraping Frameworks (Patterns Only)

### gocolly/colly ★25,100
https://github.com/gocolly/colly

**Steal these patterns**:
- **`LimitRule`**: Per-domain rate limiting with configurable delay, parallelism, and random delay variation
  ```go
  c.Limit(&colly.LimitRule{
      DomainGlob:  "*twitter.com*",
      Delay:       2 * time.Second,
      RandomDelay: 1 * time.Second,
      Parallelism: 2,
  })
  ```
- **`proxy.RoundRobinProxySwitcher`**: Simplest reliable atomic counter rotation
- **`extensions.RandomUserAgent()`**: UA rotation middleware
- **`extensions.Referer()`**: Auto-set Referer from previous page

**Files to study**:
- Rate limiting: `limitrule.go`
- Proxy rotation: `proxy/proxy.go`
- Extensions: `extensions/`

---

### geziyor/geziyor ★2,771
https://github.com/geziyor/geziyor

**Steal these patterns**:
- **Delay randomization**: `delay * (0.5 + rand.Float64())` — 0.5x to 1.5x base delay
- **Per-domain concurrent request limits**: Separate from global concurrency
- **Request deduplication**: URL hash set to avoid duplicate requests

---

## Rate Limiting

### mennanov/limiters ★621
https://github.com/mennanov/limiters

**Status**: Active (Feb 2026).

**Steal these patterns**:
- **Token bucket algorithm**: Smoother request distribution than sliding window. Better for anti-ban because requests are spaced more evenly.
- **Leaky bucket**: Fixed output rate regardless of input burst
- **Distributed backends**: Redis, etcd, DynamoDB, Memcached implementations
- **Lock interface**: Abstraction for distributed locking

**When to upgrade**: When go-stealth needs multi-instance rate limiting (Phase 5).

**Files to study**:
- Token bucket: `tokenbucket.go`
- Redis backend: `redis.go`
- Lock interface: `lock.go`

---

## Proxy Management

### pingc0y/go_proxy_pool ★779
https://github.com/pingc0y/go_proxy_pool

**Concept**: Full proxy infrastructure — discovers free proxies from multiple sources, validates them, serves via API.

**Relevant ideas**:
- Proxy validation with latency measurement
- Automatic proxy discovery from free proxy lists
- Health-based proxy scoring (not just on/off)
- REST API for proxy retrieval

**Applicability**: Low for direct integration, but health scoring pattern is useful.

---

### yunginnanet/prox5 ★81
https://github.com/yunginnanet/prox5

**Concept**: SOCKS5/4/4a validating proxy pool with upstream server.

**Relevant ideas**:
- Validation pipeline (connect → auth → target test)
- Automatic protocol detection (SOCKS4 vs SOCKS5)
- Upstream SOCKS5 proxy server (expose validated proxies)

---

## Browser Automation (Reference Only)

### playwright-community/playwright-go ★3,203
https://github.com/playwright-community/playwright-go

### go-rod/rod ★6,718
https://github.com/go-rod/rod

### chromedp/chromedp ★12,743
https://github.com/nicedoc/chromedp

**Not competitors** — different approach (full browser vs HTTP client). Relevant when:
- JavaScript rendering required
- Complex anti-bot (Cloudflare UAM, DataDome)
- go-stealth could add optional "browser fallback" mode

### jonfriesen/playwright-go-stealth ★44
https://github.com/jonfriesen/playwright-go-stealth

Ports puppeteer-extra-plugin-stealth evasions to playwright-go. Useful if we add browser fallback.

---

## Patterns Summary

| Pattern | Source | Priority | go-stealth Phase |
|---------|--------|----------|-----------------|
| Proxy health scoring | httpcloak, go_proxy_pool | High | 1 |
| Per-domain rate limiting | colly LimitRule | High | 1 |
| Delay randomization (0.5x-1.5x) | geziyor | Medium | 1 |
| Platform auto-detection | httpcloak `runtime.GOOS` | Medium | 1 |
| Priority middleware | surf | High | 2 |
| `http.RoundTripper` interface | spoofed-round-tripper | High | 2 |
| `Std()` client conversion | surf | High | 2 |
| Debug/dump mode | azuretls | Medium | 2 |
| Per-OS browser profiles | surf | Medium | 3 |
| Session abstraction | httpcloak | Medium | 3 |
| Proxy chaining (multi-hop) | azuretls | Low | 3 |
| Client Hints auto-detection | httpcloak | Low | 3 |
| Token bucket algorithm | mennanov/limiters | Low | 5 |
| Distributed rate limiting (Redis) | mennanov/limiters | Low | 5 |
| Session cache backend | httpcloak | Low | 5 |
| TLS backend abstraction | — | High | 4 |

---

## How to Use This Document

1. **Before implementing a feature** — check if a reference library has a proven pattern
2. **Before adding a dependency** — check if the pattern can be extracted instead
3. **When comparing approaches** — use the feature matrix in `compare.md`
4. **When planning releases** — use the phase mapping in `roadmap.md`
