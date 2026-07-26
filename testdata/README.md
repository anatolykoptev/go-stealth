# testdata — fingerprint oracle references

This directory holds reference fingerprint JSON files used by the
`//go:build fingerprint` oracle test (`fingerprint_oracle_test.go`).

Each file `reference_chrome_<major>.json` is the fingerprint of a **real
Chrome-for-Testing** build of that major version, captured against two echo
endpoints by the capture tool:

```
go run ./cmd/fingerprint-capture -major 146
go run ./cmd/fingerprint-capture -major 144
```

The capture tool downloads Chrome-for-Testing **linux64** (an x86_64 build). It
does **not** run on aarch64 hosts — Chrome-for-Testing has no linux-arm64 build.
On an aarch64 box, capture references on an amd64 host and commit the JSON here.

## Per-metric provenance

The oracle does not compare every metric against the same service. Echo services
disagree on how they compute hashes, and the disagreements are deterministic
(see the `internal/fingerprint` package doc for the full rationale):

- **JA4** (and `ja4_o`, `ja3n_hash`) — sourced from **browserleaks**
  (`https://tls.browserleaks.com/json`). browserleaks is FoxIO-faithful: it
  keeps the padding extension (0x0015) in JA4's third component, matching the
  FoxIO-LLC/ja4 reference implementation. peet.ws strips it — non-compliant.
- **JA3**, **peetprint**, **HTTP/2 Akamai fingerprint**, **header order**,
  **sec-ch-ua** — sourced from **peet** (`https://tls.peet.ws/api/all`). peet is
  spec-faithful for JA3 (record `legacy_version` = 771 per RFC 8446 §4.1.2) and
  is the only service that exposes peetprint and the HTTP/2 sent-frames.

Every reference records which service produced each metric in the `sources`
field. The oracle enforces that a reference and a measurement for the same
metric come from the same service — a cross-service comparison FAILs with that
stated as the reason, because it reports a tooling artefact as a fingerprint
defect.

Every reference also records its capture provenance: `browser`,
`browser_version`, `major`, `capture_time`, `endpoint`, `mode`
(headless/headful), `arch`, and `browser_source`. A reference without
provenance is unfalsifiable later.

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
