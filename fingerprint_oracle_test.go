//go:build fingerprint

// The fingerprint oracle measures whether go-stealth's emitted TLS/HTTP2/header
// fingerprint matches a real Chrome's. It is build-tagged so it NEVER runs in
// `make preflight` (which uses the default build tags) — run it explicitly:
//
//	make fingerprint
//
// A failure means one of go-stealth's Chrome profiles emits a fingerprint that
// differs from what a real Chrome of the same major version emits. That is a
// true result, not a defect in the test: a stale or wrong profile SHOULD fail
// the oracle. Fix the profile in a separate reviewed change; do not weaken the
// comparison to make it green.
//
// References live in testdata/reference_chrome_<major>.json, produced by
// `go run ./cmd/fingerprint-capture -major <major>`. An unreachable endpoint or
// a missing reference is an ERROR (never a skip) — a skip looks identical to a
// pass, which is the exact failure class this repo keeps hitting.
//
// # Per-metric provenance
//
// JA4 (and JA4_o / JA3n) are measured against browserleaks (FoxIO-faithful);
// JA3, peetprint, HTTP/2 Akamai, header order, and sec-ch-ua are measured
// against peet. Each metric in the reference and the observed carries its
// source service in Sources. Compare FAILs if a reference and a measurement
// for the same metric come from different services — a cross-service
// comparison reports a tooling artefact as a fingerprint defect.
package stealth_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-stealth"
	"github.com/anatolykoptev/go-stealth/internal/fingerprint"
)

// chromeHeaders mirrors stealth.ChromeHeaders but with a pinned profile UA
// instead of a random one, so the oracle is deterministic per profile.
func chromeHeaders(ua string) map[string]string {
	return map[string]string{
		"accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		"accept-language": "en-US,en;q=0.9",
		"accept-encoding": "gzip, deflate, br, zstd",
		"user-agent":      ua,
	}
}

// TestFingerprintOracle builds a go-stealth client for each unique Chrome TLS
// profile, requests both echo endpoints (peet for JA3/peetprint/HTTP2/headers,
// browserleaks for JA4/JA3n/JA4_o), merges the per-metric results, and compares
// the observed fingerprint against the stored reference for the same major.
func TestFingerprintOracle(t *testing.T) {
	testdataDir := "testdata"

	// Dedupe by TLSProfile: the TLS/HTTP2 fingerprint is per-major, not per-OS,
	// so testing the linux variant of each major covers every Chrome profile
	// without redundant requests. The linux variant's UA/sec-ch-ua also matches
	// the linux64 Chrome-for-Testing reference for those fields.
	type caseSpec struct {
		profile stealth.BrowserProfile
	}
	var cases []caseSpec
	seen := map[stealth.TLSProfile]bool{}
	for _, p := range stealth.BuiltinProfiles {
		if p.Browser != "chrome" || p.OS != "linux" {
			continue
		}
		if seen[p.TLSProfile] {
			continue
		}
		seen[p.TLSProfile] = true
		cases = append(cases, caseSpec{p})
	}
	if len(cases) == 0 {
		t.Fatal("no Chrome/linux profiles found in BuiltinProfiles")
	}

	for _, c := range cases {
		c := c
		t.Run(string(c.profile.TLSProfile), func(t *testing.T) {
			major := strings.TrimPrefix(string(c.profile.TLSProfile), "chrome_")
			refPath := fingerprint.ReferencePath(testdataDir, major)
			ref, err := fingerprint.LoadReference(refPath)
			if err != nil {
				// Missing reference = ERROR, not skip. A skip looks like a pass.
				if os.IsNotExist(err) {
					t.Fatalf("no reference for major %s at %s: run `go run ./cmd/fingerprint-capture -major %s` to capture one",
						major, refPath, major)
				}
				t.Fatalf("load reference %s: %v", refPath, err)
			}

			if ref.Major != major {
				t.Fatalf("reference major mismatch: reference is %s, profile is %s — wrong reference file", ref.Major, major)
			}

			obs := captureWithClient(t, c.profile)
			diffs, skipped := fingerprint.Compare(obs, ref)
			for _, s := range skipped {
				t.Logf("SKIP %s", s)
			}
			for _, d := range diffs {
				t.Errorf("FIELD %s\n  expected (real %s %s): %s\n  observed (go-stealth %s): %s",
					d.Field, ref.Browser, ref.BrowserVersion, d.Expected, c.profile.TLSProfile, d.Observed)
			}
			if len(diffs) > 0 {
				t.Logf("provenance: %s, mode=%s, arch=%s, captured=%s, endpoint=%s, sources=%+v",
					ref.BrowserSource, ref.Mode, ref.Arch, ref.CaptureTime.Format(time.RFC3339), ref.Endpoint, ref.Sources)
			}
		})
	}
}

// captureWithClient builds a go-stealth client with the given profile, sends a
// GET to both echo endpoints with Chrome-like headers, and returns the merged
// observed fingerprint. An unreachable endpoint is a fatal error (not a skip).
func captureWithClient(t *testing.T, p stealth.BrowserProfile) fingerprint.Observed {
	t.Helper()

	headers := chromeHeaders(p.UserAgent)
	for k, v := range stealth.ClientHintsHeaders(p.UserAgent) {
		headers[k] = v
	}

	client, err := stealth.NewClient(stealth.WithProfile(p.TLSProfile), stealth.WithTimeout(30))
	if err != nil {
		t.Fatalf("NewClient(%s): %v", p.TLSProfile, err)
	}

	// peet: JA3, peetprint, HTTP/2 akamai, header order, sec-ch-ua.
	peetBody, _, peetStatus, err := client.Do("GET", fingerprint.PeetEndpoint, headers, nil)
	if err != nil {
		t.Fatalf("peet endpoint unreachable for %s (this is an ERROR, not a skip): %v", p.TLSProfile, err)
	}
	if peetStatus != 200 {
		t.Fatalf("peet returned status %d for %s (expected 200)", peetStatus, p.TLSProfile)
	}
	peetRaw, err := fingerprint.ParseRaw(peetBody)
	if err != nil {
		t.Fatalf("parse peet response for %s: %v", p.TLSProfile, err)
	}
	peetObs, err := fingerprint.ExtractPeet(peetRaw)
	if err != nil {
		t.Fatalf("extract peet observed for %s: %v", p.TLSProfile, err)
	}

	// browserleaks: JA3, JA3n, JA4, JA4_o, HTTP/2 akamai (FoxIO-faithful JA4).
	blBody, _, blStatus, err := client.Do("GET", fingerprint.BrowserleaksEndpoint, headers, nil)
	if err != nil {
		t.Fatalf("browserleaks endpoint unreachable for %s (this is an ERROR, not a skip): %v", p.TLSProfile, err)
	}
	if blStatus != 200 {
		t.Fatalf("browserleaks returned status %d for %s (expected 200)", blStatus, p.TLSProfile)
	}
	blRaw, err := fingerprint.ParseRaw(blBody)
	if err != nil {
		t.Fatalf("parse browserleaks response for %s: %v", p.TLSProfile, err)
	}
	blObs, err := fingerprint.ExtractBrowserleaks(blRaw)
	if err != nil {
		t.Fatalf("extract browserleaks observed for %s: %v", p.TLSProfile, err)
	}

	obs := fingerprint.MergeObserved(peetObs, blObs)

	// Sanity: confirm the request actually went out as h2 with our UA, so a
	// silent downgrade (e.g. http/1.1 with no sent_frames) doesn't pass as
	// "all fields matched because none were extracted".
	if obs.HTTP2AkamaiFingerprint == "" {
		t.Fatalf("no HTTP/2 akamai fingerprint in merged response for %s — peet body: %s, browserleaks body: %s",
			p.TLSProfile, previewJSON(peetBody), previewJSON(blBody))
	}
	if obs.TLS.JA4 == "" {
		t.Fatalf("no JA4 in browserleaks response for %s — body: %s", p.TLSProfile, previewJSON(blBody))
	}
	return obs
}

func previewJSON(b []byte) string {
	var raw map[string]any
	if json.Unmarshal(b, &raw) == nil {
		if v, err := json.Marshal(raw); err == nil {
			return string(v)
		}
	}
	return string(b)
}

// TestReferenceProvenance checks that every committed reference file carries the
// provenance fields that make it falsifiable later — including per-metric
// Sources. A reference without provenance is a reference that can silently rot.
func TestReferenceProvenance(t *testing.T) {
	testdataDir := "testdata"
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	any := false
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "reference_chrome_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		any = true
		path := filepath.Join(testdataDir, e.Name())
		r, err := fingerprint.LoadReference(path)
		if err != nil {
			t.Errorf("%s: load: %v", e.Name(), err)
			continue
		}
		if r.BrowserVersion == "" || r.Major == "" || r.CaptureTime.IsZero() || r.Endpoint == "" || r.Mode == "" || r.BrowserSource == "" {
			t.Errorf("%s: missing provenance (browser=%q version=%q major=%q captured=%v endpoint=%q mode=%q source=%q)",
				e.Name(), r.Browser, r.BrowserVersion, r.Major, r.CaptureTime, r.Endpoint, r.Mode, r.BrowserSource)
		}
		// Per-metric sources: JA4 MUST be sourced from a FoxIO-faithful service
		// (browserleaks), not peet. JA3, HTTP2, peetprint, and headers must
		// each declare their source so a cross-service comparison is detectable.
		if r.Sources.JA4 == "" {
			t.Errorf("%s: missing sources.ja4 — JA4 source must be recorded (expected %q)", e.Name(), fingerprint.ServiceBrowserleaks)
		} else if r.Sources.JA4 == fingerprint.ServicePeet {
			t.Errorf("%s: sources.ja4=%q — peet is NOT FoxIO-compliant for JA4 (strips padding 0x0015); use %q",
				e.Name(), r.Sources.JA4, fingerprint.ServiceBrowserleaks)
		}
		if r.Sources.JA3 == "" {
			t.Errorf("%s: missing sources.ja3 — JA3 source must be recorded", e.Name())
		}
		if r.Sources.HTTP2 == "" {
			t.Errorf("%s: missing sources.http2 — HTTP/2 Akamai source must be recorded", e.Name())
		}
		if r.TLS.JA3Hash == "" || r.TLS.JA4 == "" || r.HTTP2AkamaiFingerprint == "" {
			t.Errorf("%s: missing fingerprint fields (ja3_hash=%q ja4=%q akamai=%q)",
				e.Name(), r.TLS.JA3Hash, r.TLS.JA4, r.HTTP2AkamaiFingerprint)
		}
	}
	if !any {
		t.Fatal("no reference_chrome_*.json files in testdata — run `go run ./cmd/fingerprint-capture -major <major>` to capture one")
	}
}
