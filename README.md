# go-stealth

[![Go 1.27+](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](go.mod)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A Go toolkit for HTTP clients that must present as a real browser: TLS fingerprinting, proxy rotation, rate limiting, middleware, session management, and a generic pool with health tracking.

It is a reusable HTTP layer, not an application: it carries no crawling or parsing logic.

## Features

- **TLS Fingerprinting** — browser profiles via [tls-client](https://github.com/bogdanfinn/tls-client): Chrome (up to `chrome_156`), Firefox, Safari, Brave and Edge, across macOS, Windows, Linux, Android and iOS; `BuiltinProfiles` lists the User-Agent paired with each
- **Default-deny SSRF guard** — a client refuses loopback, private, link-local and cloud-metadata targets at connect time and on redirects unless you replace the guard (`WithDialControl`, `WithRedirectGuard`, `WithRequestURLGuard`) or opt out with `WithoutSSRFGuard`
- **Proxy Rotation** — static list or [Webshare](https://www.webshare.io/) API, per-proxy health tracking with auto-skip
- **Rate Limiting** — per-key sliding window + per-domain limiter with wildcard matching
- **Middleware** — composable Handler/Middleware/Chain pattern (logging, retry, rate limit, client hints)
- **Retry & Backoff** — exponential backoff with jitter, retryable error detection, generic `RetryDo[T]()`
- **Generic Pool** — `Pool[T Identity]` with round-robin, health tracking, soft/permanent deactivation, cooldown
- **Sessions** — fixed profile + cookie jar, request counting, file-based persistence
- **`http.RoundTripper`** — drop-in replacement for `http.DefaultTransport`

## Install

```bash
go get github.com/anatolykoptev/go-stealth
```

## Quick Start

```go
p := stealth.RandomProfile(stealth.WithBrowser("chrome"))
client, _ := stealth.NewClient(
    stealth.WithIdentity(stealth.BrowserIdentity{BrowserProfile: p}), // TLS profile + User-Agent together
)
body, headers, status, err := client.Do("GET", "https://example.com", nil, nil)
```

### With Proxy Pool

```go
pool, _ := proxypool.NewWebshare(os.Getenv("WEBSHARE_API_KEY"))
client, _ := stealth.NewClient(
    stealth.WithProxyPool(pool),
    stealth.WithRetryOnBlock(2),
)
```

### Country Targeting

By default `NewWebshare` targets the US. Use `WebshareConfig` or the rotating constructor for finer control.

```go
// 1. Backbone with default US (existing call — unchanged)
pool, _ := proxypool.NewWebshare(apiKey)

// 2. Multi-country via API filter + username injection
pool, _ := proxypool.NewWebshareWithConfig(apiKey, proxypool.WebshareConfig{
    Countries: []string{"US", "GB", "DE"},
})

// 3. Rotating endpoint — no API key needed, Webshare rotates IPs internally
pool, _ := proxypool.NewWebshareRotating(os.Getenv("WEBSHARE_USER"), os.Getenv("WEBSHARE_PASS"), "US")
```

The pool round-robins across all entries. With multiple countries, each base proxy is duplicated per country so rotations naturally spread across geographies.

### As http.RoundTripper

```go
client, _ := stealth.NewClient()
resp, err := client.StdClient().Get("https://example.com")
```

### Middleware

```go
client, _ := stealth.NewClient()
client.Use(stealth.LoggingMiddleware)
client.Use(stealth.RetryMiddleware(stealth.DefaultRetryConfig))
client.Use(stealth.RateLimitMiddleware(ratelimit.NewDomainLimiter(
    ratelimit.DomainConfig{Domain: "*.example.com", RequestsPerWindow: 30, WindowDuration: time.Minute},
)))
```

### Rate Limiting

```go
// Rules are checked in order; the first matching rule wins.
limiter := ratelimit.NewDomainLimiter(
    ratelimit.DomainConfig{Domain: "api.example.com", RequestsPerWindow: 10, WindowDuration: time.Minute},
    ratelimit.DomainConfig{Domain: "*.example.com", RequestsPerWindow: 30, WindowDuration: time.Minute},
)
limiter.Wait(ctx, "https://api.example.com/v1/users")
```

### Generic Pool

```go
pool := pool.New(accounts, pool.Config{
    AlertHook: func(topic string, payload any) { log.Println(topic, payload) },
})
acc, err := pool.Next(func(a *Account) bool { return a.IsReady() })
```

## Packages

| Package | Purpose |
|---------|---------|
| `stealth` | BrowserClient, middleware, profiles, retry, backoff |
| `pool` | Generic `Pool[T Identity]` with health tracking |
| `proxypool` | ProxyPool interface + Static, Webshare, HealthyProxyPool |
| `ratelimit` | Per-key sliding window + per-domain limiter |
| `session` | Stateful browsing with persistence |
| `internal/fingerprint` | Reference types + oracle comparison for the fingerprint measurement |
| `cmd/fingerprint-capture` | Captures a real Chrome's fingerprint as an oracle reference |

## Fingerprint oracle

`make fingerprint` runs the TLS/HTTP2 fingerprint oracle, which checks that each
Chrome profile in `BuiltinProfiles` actually emits the fingerprint a real Chrome
of the same major version emits. It is **not** part of `make preflight` (it hits
the network and needs reference files); run it explicitly.

A **failure** means a go-stealth Chrome profile's emitted fingerprint differs
from a real Chrome's — a true result (the profile is stale or wrong), not a test
defect. Fix the profile in a separate reviewed change; do not weaken the
comparison to make it green.

The oracle compares each metric against a service that is spec-faithful for that
metric: **JA4** (and `ja4_o` / `ja3n_hash`) against **browserleaks**
(FoxIO-faithful — peet.ws strips the padding extension 0x0015 from JA4), and
**JA3**, **peetprint**, **HTTP/2 Akamai**, **header order**, and **sec-ch-ua**
against **peet** (spec-faithful JA3; the only service with peetprint and
sent-frames). Each reference records per-metric provenance in `sources`, and the
oracle FAILs if a reference and a measurement for the same metric come from
different services — a cross-service comparison reports a tooling artefact as a
fingerprint defect.

References live in `testdata/reference_chrome_<major>.json`, captured by:

```bash
go run ./cmd/fingerprint-capture -major 146   # amd64 host; no arm64 Chrome-for-Testing build
```

See `testdata/README.md` for the headless caveat and the per-metric provenance
contract.

## Fetch mode

By default a request is shaped like a top-level navigation (HTTP/2 priority
weight 256, `priority: u=0, i`). `stealth.WithFetchMode()` makes a client send
what Chrome sends for `fetch()` and XMLHttpRequest instead: weight 220 and
`priority: u=1, i` (a priority header you set yourself is kept). The mode is
per client because tls-client holds the HTTP/2 priority on the profile; build
one client of each kind if you mix navigations and fetches. It needs a Chrome
family profile and is opt-in for every profile, `chrome_131` included.

## Wire check against a real browser

`make fingerprint-wire` (build tag `wirelive`, network required, unreachable
endpoint fails) measures `chrome_156`, the default client, `chrome_152`,
fetch mode and `chrome_131` through tls.peet.ws and diffs JA4, ja4_r,
peetprint, the HTTP/2 fingerprint and priority against the captures in
`testdata/wire` (installed Google Chrome 156.0.8078.80; Chrome for Testing
131 for information). JA3 must vary across connections for the modern
profiles, as in Chrome, and stay constant for `chrome_131`. Offline,
`wire_*_test.go` capture the ClientHello and first HTTP/2 frames on a local
listener and pin the `chrome_131` wire byte for byte.

## Used By

- [go-twitter](https://github.com/anatolykoptev/go-twitter) — a Twitter/X client
- [go-threads](https://github.com/anatolykoptev/go-threads) — a Threads client

Plus private services in the same fleet: a web-search MCP server and a crypto-intelligence pipeline.

## Configuration

The library reads no configuration file. Everything is passed as options
(`NewClient(...)`, `pool.Config`, `proxypool.WebshareConfig`). The only environment variable it
reads itself is `INTERNAL_SERVICE_SECRET`, which `WithOxBrowser` sends as `X-Internal-Secret` to
the ox-browser service when set (it is not sent through an external proxy). Names such as
`WEBSHARE_API_KEY` in the examples are the caller's own.

## Development

```sh
make preflight           # gofmt -s, go vet, build, race tests, then three non-race live tests
make lint                # golangci-lint
make fingerprint         # TLS/HTTP2 oracle, network, not part of preflight
make fingerprint-wire    # wire check against real-browser captures, network
```

See [CONTRIBUTING.md](CONTRIBUTING.md) and, for every document in the repository,
[docs/INDEX.md](docs/INDEX.md). Releases and the changelog are produced by release-please from
Conventional Commits.

## License

Apache-2.0, see [LICENSE](LICENSE).
