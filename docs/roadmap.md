# go-stealth Roadmap

## Current State (v1.0.0)

**2685 LOC production, 2277 LOC tests** across 5 packages, 26 production files, 14 test files.

| Package | Files | LOC | Purpose |
|---------|-------|-----|---------|
| `stealth` (root) | 14 | 1526 | BrowserClient, middleware, profiles, retry, backoff, jitter, UA, proxy, backends, roundtripper |
| `pool/` | 3 | 378 | Generic identity pool with health tracking |
| `proxypool/` | 4 | 371 | ProxyPool interface, Webshare provider, health tracking with pluggable store |
| `ratelimit/` | 3 | 362 | Per-key sliding window + per-domain rate limiting with pluggable store |
| `session/` | 2 | 281 | Session management with persistence (FileStore) |

**Consumers**: go-search, go-hully (via go-twitter), go-job

**Tags**: v0.1.0 → v0.2.0 → v0.3.0 → v0.4.0 → v0.5.0 → v0.6.0 → v1.0.0

---

## Phase 1: Foundation Hardening (v0.3.0) ✅

*Proxy health, retry-after, domain rate limiting, proxypool tests, platform detection.*

- `proxypool/healthy.go` — HealthyProxyPool wrapper: per-proxy success/failure tracking, auto-skip dead proxies, cooldown with auto-reactivation, Stats(), ActiveCount()
- `retry.go` — ParseRetryAfter() for RFC 7231 delta-seconds and HTTP-date formats
- `ratelimit/domain.go` — DomainLimiter with wildcard matching, MinDelay, RandomDelay, Wait() with context
- `proxypool/webshare_test.go` — mock Webshare API tests
- `profile.go` — PlatformMatchedProfile() matching runtime.GOOS

---

## Phase 2: Middleware & Extensibility (v0.4.0) ✅

*Middleware chain, http.RoundTripper, debug mode.*

- `middleware.go` — Request, Response, Handler, Middleware types, Chain() composition
- `middleware_builtin.go` — LoggingMiddleware, RetryMiddleware, RetryMiddlewareWithContext, RateLimitMiddleware, RateLimitMiddlewareWithContext, ClientHintsMiddleware
- `client.go` — BrowserClient.Use() for middleware registration, lazy handler chain building
- `roundtripper.go` — http.RoundTripper interface, StdClient() for net/http compatibility
- `client_options.go` — WithDebug() auto-adds LoggingMiddleware

---

## Phase 3: Advanced Browser Impersonation (v0.5.0) ✅

*18 per-OS profiles, Client Hints middleware, session management.*

- `profile.go` — 18 profiles (Chrome Win/Mac/Linux/Android, Safari Mac/iOS, Firefox Win/Mac/Linux, Edge Win/Mac). RandomProfile() with WithOS/WithBrowser/WithMobile filters. ClientHintsHeaders() for mobile and Edge
- `middleware_builtin.go` — ClientHintsMiddleware auto-injects sec-ch-ua-* headers, skips Safari/Firefox
- `session/session.go` — Session struct with ID, timestamps (atomic), request counting, fixed profile + cookie jar. New(), Do(), Profile(), Age(), IdleTime()

---

## Phase 4: Pluggable TLS Backend (v0.6.0) ✅

*HTTPDoer interface, TLSProfile type, stdlib backend.*

- `backend.go` — HTTPDoer interface, BackendConfig, BackendFactory, TLSProfile string type with constants (ProfileChrome131, ProfileFirefox133, etc.)
- `backend_tlsclient.go` — bogdanfinn/tls-client wrapper. ALL tls-client/fhttp imports isolated here. Profile mapping via profileMap
- `backend_std.go` — net/http fallback. No fingerprinting, uses standard cookiejar. For testing and CGO-free environments
- `client_options.go` — WithBackend(), WithStdHTTP(), WithHTTP3(), WithProfile(TLSProfile)
- `middleware.go` — Request.HeaderOrder field, backends handle ordering internally

**Breaking changes from v0.5.0**: WithProfile() takes `TLSProfile` (string) instead of `profiles.ClientProfile` (struct). WithCookieJar() removed. BrowserProfile.TLSProfile type changed.

---

## Phase 5: Distributed Operations (v1.0.0) ✅

*Pluggable storage interfaces for distributed deployments.*

- `session/store.go` — SessionStore interface (Save/Load/List/Delete) + FileStore (JSON files on disk)
- `ratelimit/store.go` — Store interface (Increment/Count/SetBlocked/GetBlocked) + in-memory default. Limiter accepts WithStore() option
- `proxypool/healthstore.go` — HealthStore interface (Get/Set/All) + in-memory default. HealthyProxyPool accepts WithHealthStore() option

All interfaces enable Redis/etcd backing without adding dependencies to go-stealth. In-memory stores are the default — existing behavior unchanged.

---

## v2.0.0: Secure-by-Default TLS Verification

*Breaking change: TLS certificate verification is now enabled by default. The tls-client backend no longer unconditionally calls `WithInsecureSkipVerify()`.*

### Breaking change

**SECURITY: TLS certificate verification is now enabled by default.** The tls-client backend no longer unconditionally calls `WithInsecureSkipVerify()`. Consumers connecting to servers with self-signed or invalid certificates must add `stealth.WithInsecureSkipVerify()` explicitly.

### New option

- **`WithInsecureSkipVerify()`** — opt-in option that disables TLS certificate verification. Emits a `slog.Warn` at configuration time. Use only for local testing (`httptest.NewTLSServer`) or explicit MITM proxy inspection. Never use in production against public endpoints.

### WithBackend() escape hatch

Consumers needing custom TLS configuration (private PKI, custom CA bundles) should use `WithBackend()` with a custom `BackendFactory` that configures `tls.Config.RootCAs`. Custom-factory consumers are responsible for their own TLS security.

### Migration guide

| | Before (v1.x) | After (v2.0.0) |
|---|---|---|
| `stealth.NewClient()` | TLS verification **disabled** (insecure) | TLS verification **enabled** (secure) |
| Restore old behavior | — | `stealth.NewClient(stealth.WithInsecureSkipVerify())` |

**Before:**
```go
client := stealth.NewClient() // TLS verification disabled (insecure)
```

**After:**
```go
client := stealth.NewClient() // TLS verification enabled (secure)

// To restore old behavior:
client := stealth.NewClient(stealth.WithInsecureSkipVerify())
```

---

## Architecture

```
go-stealth/
├── backend.go              # HTTPDoer interface, TLSProfile, BackendConfig
├── backend_tlsclient.go    # bogdanfinn/tls-client (default backend)
├── backend_std.go          # net/http fallback
├── client.go               # BrowserClient: middleware chain, proxy rotation
├── client_options.go       # Functional options: WithProxy, WithProfile, WithStdHTTP, etc.
├── middleware.go            # Request/Response/Handler/Middleware types, Chain()
├── middleware_builtin.go   # Logging, Retry, RateLimit, ClientHints middlewares
├── roundtripper.go         # http.RoundTripper + StdClient()
├── profile.go              # 18 BrowserProfiles, RandomProfile(), ClientHintsHeaders()
├── retry.go                # RetryDo[T](), RetryConfig, ParseRetryAfter()
├── backoff.go              # BackoffConfig, exponential with jitter
├── jitter.go               # Jitter{Min,Max}, Sleep(ctx)
├── proxy.go                # MaskProxy(), ValidateProxy()
├── ua.go                   # RandomUserAgent(), ChromeHeaders()
├── pool/
│   ├── pool.go             # Pool[T Identity] — generic round-robin with health
│   ├── identity.go         # Identity interface, HealthTracker
│   └── options.go          # PoolConfig
├── proxypool/
│   ├── proxypool.go        # ProxyPool interface
│   ├── webshare.go         # Webshare API provider
│   ├── healthy.go          # HealthyProxyPool wrapper
│   └── healthstore.go      # HealthStore interface + in-memory impl
├── ratelimit/
│   ├── ratelimit.go        # Limiter — per-key sliding window
│   ├── domain.go           # DomainLimiter — per-domain with wildcards
│   └── store.go            # Store interface + in-memory impl
└── session/
    ├── session.go           # Session — stateful browsing context
    └── store.go             # SessionStore interface + FileStore (JSON)
```

---

## Key Interfaces

```go
// TLS backend (Phase 4)
type HTTPDoer interface {
    Do(req *Request) (*Response, error)
    SetProxy(url string) error
    GetCookieValue(rawURL, name string) string
}

// Rate limit storage (Phase 5)
type Store interface {
    Increment(key string, window time.Duration) (count int, windowStart time.Time)
    Count(key string, window time.Duration) (count int, windowStart time.Time)
    SetBlocked(key string, until time.Time)
    GetBlocked(key string) time.Time
}

// Proxy health storage (Phase 5)
type HealthStore interface {
    Get(proxy string) (ProxyHealth, bool)
    Set(proxy string, h ProxyHealth)
    All() map[string]ProxyHealth
}

// Session persistence (Phase 5)
type SessionStore interface {
    Save(s *Session) error
    Load(id string) (*Session, error)
    List() ([]string, error)
    Delete(id string) error
}
```

---

## Non-Goals

- **Full scraping framework** — we're a toolkit, not colly/geziyor
- **Headless browser** — out of scope, use playwright-go/chromedp/rod
- **Cloudflare solver** — too target-specific, consumer responsibility
- **Multi-lang FFI** — Go-only for now
- **JavaScript rendering** — different layer entirely
- **Redis dependency** — storage interfaces enable it, but the module stays dependency-free
