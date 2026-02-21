# Phase 4: Pluggable TLS Backend (v0.6.0)

## Goal

Abstract the TLS engine behind an interface so consumers can swap backends (bogdanfinn/tls-client, net/http, future engines) without changing business code. Decouple `profiles.ClientProfile` type from the public API.

## Architecture: Flat (Root Package)

All backends live in the root `stealth` package. The `bogdanfinn/tls-client` dependency stays in `go.mod` (it's already there). The interface allows consumers to override the backend or implement custom ones.

## New Types

### TLSProfile

Own type replacing `profiles.ClientProfile` in the public API:

```go
type TLSProfile string

const (
    ProfileChrome131    TLSProfile = "chrome_131"
    ProfileChrome133    TLSProfile = "chrome_133"
    ProfileFirefox133   TLSProfile = "firefox_133"
    ProfileSafari16     TLSProfile = "safari_16_0"
    ProfileSafariIOS18  TLSProfile = "safari_ios_18_0"
    ProfileSafariIOS17  TLSProfile = "safari_ios_17_0"
)
```

Same string values as bogdanfinn's constants — mapping is a direct type cast.

### HTTPDoer Interface

```go
type HTTPDoer interface {
    Do(req *Request) (*Response, error)
    SetProxy(url string) error
    GetCookies(u *url.URL) []*http.Cookie
}
```

### BackendConfig

```go
type BackendConfig struct {
    Profile         TLSProfile
    ProxyURL        string
    TimeoutSeconds  int
    FollowRedirects bool
    CookieJar       http.CookieJar
    HTTP3           bool
}
```

### BackendFactory

```go
type BackendFactory func(cfg BackendConfig) (HTTPDoer, error)
```

## Request Change

`Request` gains `HeaderOrder` field so backends handle ordering internally:

```go
type Request struct {
    Method      string
    URL         string
    Headers     map[string]string
    Body        io.Reader
    HeaderOrder []string  // backend applies this
}
```

## Backends

### TLS-Client Backend (`backend_tlsclient.go`)

Default backend. Wraps bogdanfinn/tls-client. ALL bogdanfinn imports (`fhttp`, `tls_client`, `profiles`) live here only — nowhere else in the codebase.

Responsibilities:
- Map `TLSProfile` to `profiles.ClientProfile`
- Build `fhttp.Request` with `HeaderOrderKey`
- Manage tls-client `HttpClient` lifecycle
- Cookie jar via `tls_client.NewCookieJar()` when none provided

### Stdlib Backend (`backend_std.go`)

Fallback using `net/http`. No TLS fingerprinting — profile is ignored. Useful for:
- Unit tests that don't need fingerprinting
- Environments where CGO is unavailable
- Quick prototyping

Responsibilities:
- Standard `http.Client` with configurable transport
- `net/http/cookiejar` for cookies
- Proxy via `http.Transport.Proxy`
- Redirect control via `CheckRedirect`

## Client Options

New options:
```go
func WithBackend(factory BackendFactory) ClientOption   // custom backend
func WithStdHTTP() ClientOption                          // convenience: stdlib backend
func WithHTTP3() ClientOption                            // QUIC (tls-client only)
```

Changed types:
```go
func WithProfile(p TLSProfile) ClientOption              // was profiles.ClientProfile
func WithCookieJar(jar http.CookieJar) ClientOption      // was tls_client.CookieJar
```

## Client Refactoring

`BrowserClient.client` (type `tls_client.HttpClient`) becomes `BrowserClient.doer` (type `HTTPDoer`).

`baseHandler()` simplifies from building fhttp.Request + reading response to:
```go
func (bc *BrowserClient) baseHandler(order []string) Handler {
    return func(req *Request) (*Response, error) {
        req.HeaderOrder = order
        return bc.doer.Do(req)
    }
}
```

`NewClient()` builds `BackendConfig` from options, calls factory, stores `HTTPDoer`.

## File Changes

| File | Action | LOC |
|------|--------|-----|
| `backend.go` | NEW: interfaces, TLSProfile, BackendConfig | ~50 |
| `backend_tlsclient.go` | NEW: bogdanfinn wrapper | ~100 |
| `backend_std.go` | NEW: net/http fallback | ~80 |
| `client.go` | REFACTOR: use HTTPDoer, simplify baseHandler | -40 |
| `client_options.go` | MODIFY: new options, type changes | +20 |
| `profile.go` | MODIFY: TLSProfile type, update constants | ~10 |
| `middleware.go` | MODIFY: add HeaderOrder to Request | +1 |
| tests | NEW + MODIFY: backend tests, update existing | ~150 |

**Total: ~250 LOC new, ~80 LOC refactored**

## Breaking Changes

| What | Before | After | Consumer Impact |
|------|--------|-------|-----------------|
| `WithProfile()` arg | `profiles.ClientProfile` | `stealth.TLSProfile` | Type change, same values |
| `WithCookieJar()` arg | `tls_client.CookieJar` | `http.CookieJar` | Standard interface |
| `BrowserProfile.TLSProfile` | `profiles.ClientProfile` | `stealth.TLSProfile` | Type change, same values |

go-twitter: ~3 lines change (type substitution).
go-search: no changes (doesn't use WithProfile or WithCookieJar directly).

## Consumer API

```go
// Default (unchanged for existing code)
client, _ := stealth.NewClient()

// Explicit stdlib backend
client, _ := stealth.NewClient(stealth.WithStdHTTP())

// HTTP/3 via tls-client
client, _ := stealth.NewClient(stealth.WithHTTP3())

// Custom backend
client, _ := stealth.NewClient(stealth.WithBackend(myFactory))
```
