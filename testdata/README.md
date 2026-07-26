# testdata — fingerprint oracle references

This directory holds reference fingerprint JSON files used by the
`//go:build fingerprint` oracle test (`fingerprint_oracle_test.go`).

Each file `reference_chrome_<major>.json` is the fingerprint of a **real
Chrome-for-Testing** build of that major version, captured against
`https://tls.peet.ws/api/all` by the capture tool:

```
go run ./cmd/fingerprint-capture -major 146
go run ./cmd/fingerprint-capture -major 144
```

The capture tool downloads Chrome-for-Testing **linux64** (an x86_64 build). It
does **not** run on aarch64 hosts — Chrome-for-Testing has no linux-arm64 build.
On an aarch64 box, capture references on an amd64 host and commit the JSON here.

Every reference records its provenance: `browser`, `browser_version`, `major`,
`capture_time`, `endpoint`, `mode` (headless/headful), `arch`, and
`browser_source`. A reference without provenance is unfalsifiable later.

## Headless caveat

The default capture mode is `headless`. Headless Chrome reports
`HeadlessChrome/<ver>` and its `sec-ch-ua` brand list uses `HeadlessChrome`
instead of `Google Chrome` — so headless references **exclude** `user-agent`
and `sec-ch-ua` (recorded in `notes`). The oracle skips those fields with the
note's reason rather than silently. TLS, HTTP/2, header order, and
accept/accept-encoding/accept-language are unaffected by headless mode (same
stack) and are compared.

To capture `user-agent` and `sec-ch-ua` references, run headful under Xvfb:

```
Xvfb :99 -screen 0 1280x720x24 &
go run ./cmd/fingerprint-capture -major 146 -mode headful -display :99
```
