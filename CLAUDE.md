# CLAUDE.md

Notes for coding agents working in go-stealth: a Go library (module `github.com/anatolykoptev/go-stealth`)
whose `BrowserClient` makes an HTTP client present a coherent browser identity (TLS and HTTP/2
fingerprint, User-Agent, Client Hints, header order), with proxy rotation, rate limiting, retry,
middleware, sessions and a generic health-tracked pool. Other fleet services depend on it, so a
change here moves their wire behaviour. Every document is indexed in [docs/INDEX.md](docs/INDEX.md).

## Layout

- Root package `stealth`: `client*.go` (BrowserClient and options), `backend*.go` (the `HTTPDoer`
  interface; tls-client and plain net/http backends), `profile.go` and `ua.go` (profiles and
  identities), `extension_order.go`, `header_order.go`, `decompress.go`, `ssrf.go` and `proxy*.go`
  (guards), `retry*.go`, `backoff.go`, `middleware*.go`, `cloudflare.go`, `solver_*.go`,
  `oxbrowser.go`, `site_intel.go`.
- Subpackages: `pool/` (generic `Pool[T Identity]`), `proxypool/` (static, Webshare, healthy
  wrapper), `ratelimit/`, `session/`, `internal/fingerprint` and `internal/uri`.
- `cmd/fingerprint-capture`: records a real Chrome's fingerprint as an oracle reference in
  `testdata/`. `testdata/wire/` holds real-browser captures for the wire check.
- `docs/`: `compare.md`, `references.md` and `roadmap.md`.

## Gate

CI (`.github/workflows/preflight.yml`, job `preflight`, `ubuntu-24.04-arm`, Go `stable`) runs
`make preflight`: `gofmt -s -l` must print nothing, `go vet ./...`, `go build ./...`,
`go test -race -count=1 ./...`, then three `TestLive_*` tests again without `-race` and fails if
any of them did not run. Always set `GOWORK=off` (the Makefile does). `make lint` runs
golangci-lint and is not in CI. `make fingerprint` and `make fingerprint-wire` use the network,
are not in preflight and must not be added to it. A docs-only PR (Markdown, `docs/`, licence,
issue and PR templates) skips the `preflight` job through the `docs-scope` job in the same
workflow; that block is canonical across the fleet and compared byte for byte, so do not edit it.

## Invariants that are easy to break

- **A profile's TLS fingerprint, User-Agent and Client Hints must describe one browser.**
  `BuiltinProfiles` is pinned by `TestBuiltinProfilesCoherent`; do not add a UA newer than the TLS
  profile has been measured against. `chrome_156` reuses tls-client's Chrome 152 profile because
  it was measured identical; re-measure the same way (`make fingerprint-wire`) before adding a major.
- **`chrome_131` is frozen.** Existing consumers pin it; `wire_chrome131_frozen_test.go` fixes its
  wire bytes. A tls-client bump or a client-wide option meant for newer profiles must not move it.
- **A fingerprint-oracle failure is a true result.** Fix the profile in a separate reviewed change;
  never loosen the comparison to turn it green. The oracle compares each metric with the service
  that is spec-faithful for it, and fails when a reference and a measurement for one metric come
  from different services (`testdata/README.md`).
- **The SSRF guard is default-deny and fail-closed by construction.** A client with no guard config
  refuses loopback, private, link-local and metadata addresses (connect time, redirects, request
  URL). Guards set through `WithDialControl`, `WithRedirectGuard`, `WithRequestURLGuard` are
  tagged so `errors.Is(err, ErrSSRFBlocked)` holds and retry does not re-run a blocked target.
  Do not add a code path that skips the guards without `WithoutSSRFGuard`.
- **Public option signatures stay free of tls-client and fhttp types**; those live behind
  `HTTPDoer` and `BackendConfig` so the engine can be swapped.
- **The `INTERNAL_SERVICE_SECRET` header is not sent to ox-browser through an external proxy**, and
  `WithInsecureSkipVerify` keeps its loud warning. Keep both.
- **Races are tested.** Client `SetProxy`, `Close`, `CloseIdleConnections` and the pool are used
  concurrently by consumers; keep the `-race` tests and the non-race live tests green.
- **Versions come from release-please**, which reads Conventional Commits. Never tag or edit
  `CHANGELOG.md` or `.release-please-manifest.json` by hand.
- **This repository is public.** No hostnames, operator paths, account names, tokens or personal
  data in any tracked file, tests, fixtures and captures included. The existing findings listed
  by `repo-standards check` in `docs/` and `ssrf_test.go` are known cleanup, not a licence to add more.
