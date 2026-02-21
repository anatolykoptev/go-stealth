# go-stealth Roadmap

## Current State (v0.2.0)

**769 LOC production code** across 4 packages:

| Package | LOC | Purpose |
|---------|-----|---------|
| `stealth` (root) | 501 | BrowserClient, retry, backoff, jitter, profiles, UA |
| `pool/` | 317 | Generic identity pool with health tracking |
| `ratelimit/` | 126 | Per-key sliding window rate limiter |
| `proxypool/` | 107 | ProxyPool interface + Webshare provider |

**Consumers**: go-search, go-hully (via go-twitter), go-job

---

## Phase 1: Foundation Hardening (v0.3.0)

*Goal: Fix known gaps, improve reliability.*

### 1.1 Proxy Health Tracking
**Source**: httpcloak's connection pool health checks

Add health monitoring to `ProxyPool`:
- Track success/failure rate per proxy
- Auto-skip dead proxies (configurable threshold)
- `ProxyPool.Stats()` — success rate, avg latency per proxy
- Cooldown for failing proxies with auto-reactivation

```go
type HealthyProxyPool struct {
    pool      ProxyPool
    tracker   map[string]*ProxyHealth  // per-proxy stats
    threshold float64                   // failure rate to deactivate
}
```

### 1.2 Retry-After Header Parsing
**Source**: RFC 7231

When receiving 429, parse `Retry-After` header and integrate with `Limiter.MarkRateLimited()`:

```go
func ParseRetryAfter(resp *http.Response) time.Duration
```

### 1.3 Per-Domain Rate Limiting
**Source**: colly's `LimitRule`

Extend `ratelimit.Limiter` with domain-aware limiting:

```go
type DomainConfig struct {
    Domain           string
    RequestsPerWindow int
    WindowDuration   time.Duration
    Delay            time.Duration  // min delay between requests
}
```

### 1.4 Proxypool Tests
**Current gap**: 0% test coverage for proxypool package.
- Mock Webshare API responses
- Test round-robin rotation
- Test error handling (empty response, auth failure)

### 1.5 Platform Auto-Detection
**Source**: httpcloak's `runtime.GOOS`

Match User-Agent OS to actual platform:
```go
func PlatformMatchedProfile() BrowserProfile  // picks Chrome-Windows on Windows, etc.
```

---

## Phase 2: Middleware & Extensibility (v0.4.0)

*Goal: Make go-stealth extensible without forking.*

### 2.1 Request/Response Middleware
**Source**: surf's priority-ordered middleware system

```go
type Middleware func(next Handler) Handler
type Handler func(req *Request) (*Response, error)

func (bc *BrowserClient) Use(mw ...Middleware)
```

Built-in middleware:
- `LoggingMiddleware` — request/response logging
- `MetricsMiddleware` — success/failure counters per domain
- `RetryMiddleware` — configurable retry with backoff
- `RateLimitMiddleware` — domain-aware rate limiting

### 2.2 http.RoundTripper Interface
**Source**: spoofed-round-tripper, surf's `Std()`

Expose `BrowserClient` as standard `http.RoundTripper`:
```go
func (bc *BrowserClient) RoundTrip(req *http.Request) (*http.Response, error)
func (bc *BrowserClient) StdClient() *http.Client
```

This enables using go-stealth with any Go HTTP library (resty, go-retryablehttp, etc.).

### 2.3 Request Context & Debug Mode
**Source**: azuretls's request dump

```go
func WithDebug() ClientOption          // enable request/response logging
func WithRequestID() Middleware        // attach unique ID to each request
```

---

## Phase 3: Advanced Browser Impersonation (v0.5.0)

*Goal: Match surf/httpcloak's impersonation quality.*

### 3.1 Per-OS Browser Profiles
**Source**: surf's per-OS variants

Expand from 8 flat profiles to OS-specific variants:

| Browser | Windows | macOS | Linux | Android | iOS |
|---------|---------|-------|-------|---------|-----|
| Chrome 131+ | yes | yes | yes | yes | - |
| Safari 18+ | - | yes | - | - | yes |
| Firefox 133+ | yes | yes | yes | - | - |
| Edge 131+ | yes | yes | - | - | - |

```go
func RandomProfile(opts ...ProfileOption) BrowserProfile
func WithOS(os string) ProfileOption       // "windows", "macos", "linux", "android", "ios"
func WithBrowser(b string) ProfileOption   // "chrome", "firefox", "safari", "edge"
```

### 3.2 Client Hints Auto-Detection
**Source**: httpcloak's Accept-CH parsing

When server sends `Accept-CH` header, auto-generate matching Client Hints:
```go
func (bc *BrowserClient) DoWithHints(method, url string, headers map[string]string, body io.Reader) ([]byte, map[string]string, int, error)
```

### 3.3 Session Management
**Source**: httpcloak's session abstraction

```go
type Session struct {
    ID           string
    CreatedAt    time.Time
    LastUsed     time.Time
    RequestCount int64
    CookieJar    CookieJar
    Profile      BrowserProfile
}

func NewSession(opts ...SessionOption) *Session
func (s *Session) Do(method, url string, ...) ([]byte, map[string]string, int, error)
```

---

## Phase 4: Pluggable TLS Backend (v0.6.0)

*Goal: Support multiple TLS engines, not just bogdanfinn/tls-client.*

### 4.1 TLS Backend Interface

```go
type TLSBackend interface {
    Do(req *Request) (*Response, error)
    SetProxy(url string) error
    GetCookieValue(url, name string) string
}

// Implementations:
type TLSClientBackend struct { ... }   // bogdanfinn/tls-client (current)
type SurfBackend struct { ... }         // enetx/surf (future)
type StdBackend struct { ... }          // net/http (fallback, no fingerprint)
```

### 4.2 HTTP/3 QUIC Support
**Source**: surf, httpcloak, tls-client

If using tls-client or surf backend, enable HTTP/3:
```go
func WithHTTP3() ClientOption
```

---

## Phase 5: Distributed Operations (v1.0.0)

*Goal: Scale across multiple instances.*

### 5.1 Distributed Rate Limiting
**Source**: mennanov/limiters

Support Redis/etcd backends for rate limiting across instances:
```go
func NewDistributedLimiter(cfg Config, backend Backend) *Limiter
```

### 5.2 Distributed Proxy Health
Share proxy health scores across instances via Redis:
```go
type SharedProxyPool struct {
    local  ProxyPool
    redis  *redis.Client
    prefix string
}
```

### 5.3 Session Persistence
**Source**: httpcloak's session cache backend

Persist sessions (cookies, request count, TLS state) to disk/Redis:
```go
type SessionStore interface {
    Save(session *Session) error
    Load(id string) (*Session, error)
}
```

---

## Priority Matrix

| Feature | Impact | Effort | Phase | Consumer Need |
|---------|--------|--------|-------|---------------|
| Proxy health tracking | High | Medium | 1 | go-search (dead proxy retry) |
| Retry-After parsing | Medium | Low | 1 | all consumers |
| Per-domain rate limiting | Medium | Low | 1 | go-hully (Twitter limits) |
| Proxypool tests | Medium | Low | 1 | reliability |
| Middleware system | High | Medium | 2 | extensibility |
| RoundTripper interface | High | Low | 2 | std lib compat |
| Per-OS profiles | Medium | Medium | 3 | detection bypass |
| Session management | Medium | Medium | 3 | stateful scraping |
| Pluggable TLS backend | High | High | 4 | future-proofing |
| Distributed rate limiting | Low | High | 5 | multi-instance |

---

## Non-Goals

- **Full scraping framework** — we're a toolkit, not colly/geziyor
- **Headless browser** — out of scope, use playwright-go/chromedp/rod
- **Cloudflare solver** — too target-specific, consumer responsibility
- **Multi-lang FFI** — Go-only for now
- **JavaScript rendering** — different layer entirely
