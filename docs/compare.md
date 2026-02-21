# Competitive Analysis: go-stealth vs. Alternatives

## Market Position

go-stealth occupies a unique niche: the **operational anti-ban layer**. TLS impersonation libraries (surf, httpcloak, tls-client) solve "how to look like a browser". go-stealth solves "how to not get banned at scale" — proxy rotation, rate limiting, health tracking, retry with backoff, identity pool management.

No competitor combines all of these into a single toolkit.

## Competitor Overview

| Library | Stars | Created | Activity | Approach |
|---------|-------|---------|----------|----------|
| **enetx/surf** | 1,325 | 2025-08 | Very active | Full custom stack, fluent API |
| **bogdanfinn/tls-client** | 1,466 | 2022-07 | Active | TLS fingerprint engine (our dependency) |
| **sardanioss/httpcloak** | 750 | 2025-12 | Active | Custom stack, multi-lang FFI |
| **Noooste/azuretls-client** | 426 | 2023-05 | Moderate | Session-based, ergonomic |
| **gocolly/colly** | 25,100 | 2017-09 | Active | Scraping framework |
| **geziyor/geziyor** | 2,771 | 2019-06 | Stale | Scrapy clone for Go |
| **go-stealth** | new | 2026-02 | Active | Operational anti-ban toolkit |

---

## Feature Matrix

| Feature | go-stealth | tls-client | surf | httpcloak | azuretls | colly |
|---------|-----------|------------|------|-----------|----------|-------|
| TLS Fingerprint | via tls-client | native | native | native | native | no |
| HTTP/3 QUIC | no | yes | yes | yes | yes | no |
| Header Ordering | yes | yes | yes | yes | yes | no |
| Browser Profiles | 8 (basic) | basic | advanced (per-OS) | advanced | good | UA only |
| **Proxy Rotation** | **yes (auto)** | no | no | no | no | basic RR |
| **Rate Limiting** | **sliding window** | no | no | no | no | per-domain |
| **Retry/Backoff** | **yes + jitter** | no | yes | no | no | yes |
| **Health Tracking** | **yes (pool)** | no | no | conn-level | no | no |
| **Resource Pool** | **generic** | no | no | conn pool | no | no |
| Middleware | no | hooks | priority-based | no | no | callbacks |
| Session Mgmt | no | cookie jar | no | full session | session | no |
| Std lib compat | no | interface | Std() | no | no | net/http |
| ECH | no | no | no | yes | no | no |
| Multi-lang FFI | no | JS/Py/C# | no | Go/Py/JS/C# | CFFI | no |

**Bold** = go-stealth's unique strengths that no TLS library provides.

---

## Detailed Competitor Analysis

### 1. enetx/surf — Most Complete TLS Library

**Architecture**: Custom stack (own forks of utls, http, http2, http3). No dependency on bogdanfinn.

**Strengths over go-stealth**:
- Fluent builder API: `surf.NewClient().Builder().Impersonate().Chrome().Build()`
- Per-OS browser variants (Windows/macOS/Linux/Android/iOS)
- `RandomOS()` for fingerprint diversity
- HTTP/3 over QUIC with browser-specific fingerprinting
- HTTP/2 SETTINGS frame customization
- Priority-ordered middleware system
- `Std()` method — converts to `net/http.Client` for standard library compatibility
- `sync.Pool` for decompression (gzip, brotli, zstd)
- DNS-over-TLS

**Missing (go-stealth has)**:
- No proxy rotation (single proxy per client)
- No rate limiting
- No generic resource pool with health tracking
- No alert hooks or monitoring

**Verdict**: Best TLS impersonation. Different layer than go-stealth — complementary, not competing.

### 2. bogdanfinn/tls-client — Our Foundation

**Architecture**: Forked utls + fhttp. The most established Go TLS fingerprint library.

**Strengths**:
- HTTP/1.1, HTTP/2, HTTP/3 protocol support
- Protocol racing (Chrome-like H2/H3 negotiation)
- WebSocket support with maintained fingerprints
- Pre/post request hooks (new, Feb 2026)
- FFI bindings (Node.js, Python, C#)
- Certificate pinning, bandwidth tracking

**Missing (go-stealth adds)**:
- No proxy rotation, rate limiting, retry, health tracking, pool management

**Verdict**: Our dependency. go-stealth adds the operational layer on top.

### 3. sardanioss/httpcloak — Session-Focused

**Architecture**: Clean package separation (fingerprint/, session/, pool/, transport/, proxy/, dns/).

**Strengths over go-stealth**:
- Session management with ID, timestamps, request counting
- Distributed TLS session cache backend
- Connection pool with health checks (`IsHealthy()`, `Age()`, `IdleTime()`)
- ECH (Encrypted Client Hello) — hides SNI
- Platform auto-detection via `runtime.GOOS`
- Client Hints auto-detection from server `Accept-CH` headers
- MASQUE proxy, domain fronting

**Missing (go-stealth has)**:
- No proxy rotation, rate limiting, retry/backoff, generic resource pool

**Verdict**: Best session/connection management. Session abstraction is worth adopting.

### 4. Noooste/azuretls-client — Ergonomic

**Strengths over go-stealth**:
- Proxy chaining (multi-hop proxies)
- Zero-config Chrome impersonation by default
- Session-based API
- Request dump/debug mode
- Edge browser preset

**Missing**: No rotation, no rate limiting, no retry, no health tracking.

### 5. gocolly/colly — The Giant (25k stars)

**Different category** — full scraping framework, not anti-ban toolkit.

**Relevant patterns**:
- `proxy.RoundRobinProxySwitcher()` — simple but effective atomic counter rotation
- `LimitRule` — per-domain rate limiting with delay, parallelism, random delay
- `extensions.RandomUserAgent()` — UA rotation per request

**No TLS fingerprinting at all** — uses standard `net/http.Client`.

---

## What go-stealth Does Better Than Everyone

1. **Generic resource pool** (`pool.Pool[T Identity]`) with health tracking, soft-deactivation, auto-reactivation — no competitor has this at the generic level
2. **Proxy rotation integrated into HTTP client** (`WithProxyPool`) — the only library that auto-rotates per-request
3. **Rate limiting + retry combined** — the operational layer competitors completely lack
4. **Concrete proxy provider** (Webshare API) — not just an interface, a working integration
5. **Alert hooks** on pool events — monitoring integration none of the others offer
6. **Exported error types** (`HttpStatusError`) for cross-module retry compatibility

## What go-stealth is Missing

### Critical Gaps
1. **No HTTP/3 or QUIC fingerprinting** — becoming table stakes for bot detection
2. **No middleware system** — surf's priority-ordered middleware is the gold standard
3. **No `http.RoundTripper` interface** — can't plug into standard `net/http.Client`
4. **No session management** — no request counting, no session persistence
5. **Browser profiles are flat** — no per-OS variants, no random OS selection

### Nice-to-Have
6. No proxy health monitoring (uptime tracking, auto-removal of dead proxies)
7. No per-domain rate limiting (only per-key)
8. No Retry-After header parsing from 429 responses
9. No mobile browser profiles (iOS Safari, Android Chrome)
10. No request/response debugging mode
11. No proxy chaining (multi-hop)
12. No platform auto-detection (`runtime.GOOS`)

---

## Strategic Conclusion

go-stealth's value is **orthogonal** to TLS impersonation libraries. They solve transport-level fingerprinting; we solve operational-level anti-ban at scale. The ideal architecture:

```
[Consumer: go-search, go-hully, go-twitter, go-job]
    |
[go-stealth: proxy rotation, rate limiting, retry, pool, health tracking]
    |
[TLS backend: tls-client (current) | surf | httpcloak (future option)]
```

**Priority**: Strengthen the operational layer (middleware, session mgmt, proxy health) rather than competing on TLS fingerprinting. Consider making the TLS backend pluggable.
