# Contributing to go-stealth

go-stealth is a Go library that other services build on, so a change can alter what they send on
the wire. Open an issue before changing a profile, a default, the SSRF guard or a public option.
Report vulnerabilities privately, see [SECURITY.md](SECURITY.md).

## Development

You need the Go version in `go.mod` (1.27.3 or newer) and, for lint, golangci-lint v2.

```sh
make build               # go build ./...
make test                # go test -race -count=1 ./...
make lint                # golangci-lint run ./...
make preflight           # the CI gate: gofmt -s, go vet, build, race tests, three non-race live tests
```

CI runs `make preflight` on `ubuntu-24.04-arm` for every pull request (workflow `preflight`). If it
passes locally it passes there. A pull request that changes only documents skips that job.

Two checks need the network and are not part of the gate; run them when you touch a profile, the
header or extension order, or the HTTP/2 settings:

```sh
make fingerprint         # TLS/HTTP2 oracle against reference captures in testdata/
make fingerprint-wire    # wire check against real-browser captures in testdata/wire/
```

A failure there means the emitted fingerprint differs from a real browser's. Fix the profile in a
reviewed change; do not weaken the comparison. References are captured with
`go run ./cmd/fingerprint-capture -major <N>` on an amd64 host, see
[testdata/README.md](testdata/README.md).

## Commits

Conventional Commits: `feat:`, `fix:`, `perf:`, `refactor:`, `docs:`, `deps:`, `revert:`, and
`chore:`, `ci:`, `test:`, `build:`, `style:` (hidden from the changelog). release-please reads
them: `feat:` makes a minor release, `fix:` a patch, and `feat!:` or a `BREAKING CHANGE:` footer a
breaking one (before 1.0 it raises the minor version). A commit that does not parse is left out of
the release notes, so keep the subject clean and put code in the body. Never tag or edit
`CHANGELOG.md` by hand.

## Pull requests

Describe what changed, why, and how it was tested. A behaviour fix comes with a test that fails
without it. Keep the repository free of hostnames, operator paths, account names and tokens, test
fixtures included. Update [docs/INDEX.md](docs/INDEX.md) when you add or rename a document. By
contributing you agree your work is licensed under Apache-2.0, see [LICENSE](LICENSE).
