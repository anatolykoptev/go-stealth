// Package fingerprint holds the reference-capture and oracle infrastructure that
// measures whether go-stealth's emitted TLS/HTTP2 fingerprint matches a real
// browser's. It is the measurement layer for the "do we actually look like
// Chrome?" question — without it, profile staleness produces no failing signal.
//
// The reference files (testdata/reference_chrome_<major>.json) are produced by
// the cmd/fingerprint-capture tool, which downloads a Chrome-for-Testing build
// matching a given major, runs it against the echo endpoints, and records the
// observed fingerprint with full provenance. The oracle test (build-tagged
// //go:build fingerprint, so never in preflight) builds a go-stealth client per
// Chrome profile, hits the same endpoints, and compares observed-vs-reference
// field by field.
//
// # Per-metric provenance and the same-service invariant
//
// Fingerprint echo services disagree on how they compute hashes — and the
// disagreements are deterministic, not noise. A service study (see the commit
// message of the fix that introduced per-metric provenance) established:
//
//   - JA4: tls.peet.ws strips the padding extension (0x0015) from JA4's third
//     component, while the FoxIO reference implementation keeps it. peet's JA4
//     is therefore non-FoxIO-compliant. browserleaks and thumbprint.me both
//     match FoxIO.
//   - JA3: peet, browserleaks, and ja3.zone are all spec-faithful (record
//     legacy_version = 771 per RFC 8446 §4.1.2). Only scrapfly deviates (772).
//   - peetprint is peet-proprietary; no other service exposes it.
//
// An oracle that compares our JA4 against a non-reference implementation would
// report drift that isn't there, or miss drift that is. The compliance verdict
// is per metric, not per service. This package therefore records which service
// produced each metric (the Sources struct) and enforces that a reference and a
// measurement for the same metric come from the same service — a cross-service
// comparison reports a tooling artefact as a fingerprint defect, so Compare
// FAILs with that stated as the reason rather than silently comparing across
// services.
//
// JA4 (and the order-insensitive JA4_o / JA3n variants) come from browserleaks.
// JA3, peetprint, HTTP/2 Akamai fingerprint, header order, and sec-ch-ua come
// from peet. browserleaks additionally returns ja3n_hash and ja4_o — the
// order-insensitive variants that survive extension-order randomization (a
// future Brave profile with RandomExtensionOrder: true makes plain JA3 change
// every connection).
package fingerprint

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Echo service identifiers — recorded per metric in Sources so a future reader
// knows where each hash came from.
const (
	ServicePeet         = "peet"
	ServiceBrowserleaks = "browserleaks"
)

// PeetEndpoint is the peet.ws echo URL. peet is spec-faithful for JA3 (record
// legacy_version = 771) and is the only service that exposes peetprint and the
// HTTP/2 sent-frames (header order). It is NOT FoxIO-compliant for JA4 — it
// strips the padding extension (0x0015) from JA4's third component — so JA4 is
// sourced from browserleaks instead.
const PeetEndpoint = "https://tls.peet.ws/api/all"

// BrowserleaksEndpoint is the browserleaks echo URL. browserleaks is
// FoxIO-faithful for JA4 (keeps padding 0x0015) and spec-faithful for JA3
// (legacy_version = 771). It returns ja3n_hash and ja4_o — the order-insensitive
// variants that survive extension-order randomization. It does NOT expose
// peetprint, header order, or sec-ch-ua.
const BrowserleaksEndpoint = "https://tls.browserleaks.com/json"

// Reference is a captured browser fingerprint used as the oracle's ground truth.
// A reference without provenance is unfalsifiable later — every field below the
// blank line exists so a future reader can decide whether the reference is still
// trustworthy.
type Reference struct {
	Browser        string    `json:"browser"`         // "Chrome" (Chrome-for-Testing) or "Chromium"
	BrowserVersion string    `json:"browser_version"` // full version, e.g. "146.0.7633.0"
	Major          string    `json:"major"`           // major version, e.g. "146"
	CaptureTime    time.Time `json:"capture_time"`
	Endpoint       string    `json:"endpoint"`       // primary capture endpoint (browserleaks — the JA4 source)
	Mode           string    `json:"mode"`           // "headless" | "headful"
	Arch           string    `json:"arch"`           // runtime.GOARCH of the capture host
	BrowserSource  string    `json:"browser_source"` // "chrome-for-testing" | "system-chromium"

	// Sources records which echo service produced each metric. A reference and
	// a measurement for the same metric MUST come from the same service —
	// comparing across services reports a tooling artefact as a fingerprint
	// defect. See the package doc for why JA4 moved off peet.
	Sources Sources `json:"sources"`

	TLS                    TLSFingerprint `json:"tls"`
	HTTP2AkamaiFingerprint string         `json:"http2_akamai_fingerprint"`

	HeaderOrder     []string `json:"header_order,omitempty"`
	SecChUa         string   `json:"sec_ch_ua,omitempty"`
	SecChUaMobile   string   `json:"sec_ch_ua_mobile,omitempty"`
	SecChUaPlatform string   `json:"sec_ch_ua_platform,omitempty"`
	Accept          string   `json:"accept,omitempty"`
	AcceptLanguage  string   `json:"accept_language,omitempty"`
	AcceptEncoding  string   `json:"accept_encoding,omitempty"`
	UserAgent       string   `json:"user_agent,omitempty"`

	// Notes Records caveats that make a field non-comparable — e.g. headless
	// mode excludes UA/sec-ch-ua because headless Chrome reports
	// HeadlessChrome/<ver>, not Chrome/<ver>. The oracle reads Notes to decide
	// which fields to skip with an explicit reason rather than silently.
	Notes []string `json:"notes,omitempty"`
}

// Sources records which echo service produced each metric. Every metric that
// could come from more than one service has a field here; the value is the
// ServiceXxx constant. Compare checks that the reference and observed Sources
// agree per metric before comparing values — a mismatch is a hard FAIL, not a
// silent cross-service comparison.
type Sources struct {
	JA3       string `json:"ja3"`                 // JA3 hash source — peet or browserleaks
	JA3n      string `json:"ja3n,omitempty"`      // JA3n (sorted) hash source — browserleaks
	JA4       string `json:"ja4"`                 // JA4 source — browserleaks (FoxIO-faithful)
	JA4O      string `json:"ja4_o,omitempty"`     // JA4 original-order source — browserleaks
	Peetprint string `json:"peetprint,omitempty"` // peetprint source — always peet
	HTTP2     string `json:"http2"`               // HTTP/2 Akamai fingerprint source
	Headers   string `json:"headers,omitempty"`   // header order + sec-ch-ua source — peet
}

// TLSFingerprint holds the TLS-layer fingerprint fields the echo services
// report. peet.ws strips RFC 8701 GREASE (?A?A) values from ja3/ja3_hash/
// ja4/peetprint before hashing, so those fields are stable across connections
// of the same client and can be compared directly without further GREASE
// normalization. browserleaks does the same for ja3_hash/ja3n_hash/ja4/ja4_o.
type TLSFingerprint struct {
	JA3           string `json:"ja3"`
	JA3Hash       string `json:"ja3_hash"`
	JA3nHash      string `json:"ja3n_hash,omitempty"` // browserleaks order-insensitive JA3
	JA4           string `json:"ja4"`
	JA4O          string `json:"ja4_o,omitempty"` // browserleaks original-order JA4
	Peetprint     string `json:"peetprint,omitempty"`
	PeetprintHash string `json:"peetprint_hash,omitempty"`
}

// Observed is the fingerprint extracted from echo-service responses for a
// single request. It mirrors the comparable subset of Reference. Because
// metrics come from two services, an Observed is built by merging a peet
// extraction and a browserleaks extraction (see MergeObserved).
type Observed struct {
	Sources Sources

	TLS                    TLSFingerprint
	HTTP2AkamaiFingerprint string

	HeaderOrder     []string
	SecChUa         string
	SecChUaMobile   string
	SecChUaPlatform string
	Accept          string
	AcceptLanguage  string
	AcceptEncoding  string
	UserAgent       string
}

// ReferencePath returns the testdata path for a reference of the given major.
func ReferencePath(testdataDir, major string) string {
	return filepath.Join(testdataDir, "reference_chrome_"+major+".json")
}

// LoadReference reads a reference JSON file.
func LoadReference(path string) (*Reference, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Reference
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &r, nil
}

// SaveReference writes a reference JSON file with stable formatting.
func SaveReference(path string, r *Reference) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

// ParseRaw parses a JSON body into a raw map. Used for both peet and
// browserleaks responses (both are JSON objects).
func ParseRaw(body []byte) (map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse echo response: %w", err)
	}
	return raw, nil
}

// ExtractPeet pulls the peet-sourced metrics out of a raw peet.ws /api/all
// response map: JA3, peetprint, HTTP/2 Akamai fingerprint, header order, and
// sec-ch-ua / accept* / user_agent from the HTTP/2 HEADERS frame. The returned
// Observed has only the peet-sourced fields populated; merge with
// ExtractBrowserleaks for the full set.
func ExtractPeet(raw map[string]any) (Observed, error) {
	var o Observed

	tlsMap, _ := raw["tls"].(map[string]any)
	o.TLS.JA3, _ = tlsMap["ja3"].(string)
	o.TLS.JA3Hash, _ = tlsMap["ja3_hash"].(string)
	o.TLS.Peetprint, _ = tlsMap["peetprint"].(string)
	o.TLS.PeetprintHash, _ = tlsMap["peetprint_hash"].(string)

	o.UserAgent, _ = raw["user_agent"].(string)

	h2Map, _ := raw["http2"].(map[string]any)
	o.HTTP2AkamaiFingerprint, _ = h2Map["akamai_fingerprint"].(string)

	frames, _ := h2Map["sent_frames"].([]any)
	for _, f := range frames {
		fm, ok := f.(map[string]any)
		if !ok || fm["frame_type"] != "HEADERS" {
			continue
		}
		headers, _ := fm["headers"].([]any)
		for _, h := range headers {
			hs, _ := h.(string)
			// Pseudo-headers look like ":method: GET" — the leading colon means
			// the name is ":method", not "" (splitHeader would split at index 0).
			// Skip them by raw-prefix; their order is encoded in the akamai
			// fingerprint, not the regular header order.
			if strings.HasPrefix(hs, ":") {
				continue
			}
			name, val := splitHeader(hs)
			o.HeaderOrder = append(o.HeaderOrder, name)
			switch name {
			case "sec-ch-ua":
				o.SecChUa = val
			case "sec-ch-ua-mobile":
				o.SecChUaMobile = val
			case "sec-ch-ua-platform":
				o.SecChUaPlatform = val
			case "accept":
				o.Accept = val
			case "accept-language":
				o.AcceptLanguage = val
			case "accept-encoding":
				o.AcceptEncoding = val
			}
		}
		break // first HEADERS frame is the request
	}

	o.Sources = Sources{
		JA3:       ServicePeet,
		Peetprint: ServicePeet,
		HTTP2:     ServicePeet,
		Headers:   ServicePeet,
	}
	return o, nil
}

// ExtractBrowserleaks pulls the browserleaks-sourced metrics out of a raw
// tls.browserleaks.com/json response map: JA3 (hash), JA3n (hash), JA4, JA4_o,
// and the HTTP/2 Akamai fingerprint. The returned Observed has only the
// browserleaks-sourced fields populated; merge with ExtractPeet for the full
// set. browserleaks does NOT expose peetprint, header order, or sec-ch-ua.
func ExtractBrowserleaks(raw map[string]any) (Observed, error) {
	var o Observed

	o.TLS.JA3Hash, _ = raw["ja3_hash"].(string)
	o.TLS.JA3nHash, _ = raw["ja3n_hash"].(string)
	o.TLS.JA4, _ = raw["ja4"].(string)
	o.TLS.JA4O, _ = raw["ja4_o"].(string)
	o.HTTP2AkamaiFingerprint, _ = raw["akamai_text"].(string)

	o.Sources = Sources{
		JA3:   ServiceBrowserleaks,
		JA3n:  ServiceBrowserleaks,
		JA4:   ServiceBrowserleaks,
		JA4O:  ServiceBrowserleaks,
		HTTP2: ServiceBrowserleaks,
	}
	return o, nil
}

// MergeObserved combines a peet-sourced and a browserleaks-sourced Observed
// into a single Observed. browserleaks values take precedence for JA4, JA3n,
// and JA4_o (FoxIO-faithful + order-insensitive); peet values take precedence
// for peetprint, header order, and sec-ch-ua (browserleaks does not expose
// them). JA3 hash and HTTP/2 Akamai fingerprint come from browserleaks
// (spec-faithful + FoxIO-faithful respectively, and the same values as peet
// for a spec-faithful client). Both inputs must be non-nil.
func MergeObserved(peet, bl Observed) Observed {
	merged := peet // start with peet (peetprint, headers, sec-ch-ua, accept*)

	// Overwrite / fill in browserleaks-sourced metrics.
	merged.TLS.JA3Hash = bl.TLS.JA3Hash
	merged.TLS.JA3nHash = bl.TLS.JA3nHash
	merged.TLS.JA4 = bl.TLS.JA4
	merged.TLS.JA4O = bl.TLS.JA4O
	merged.HTTP2AkamaiFingerprint = bl.HTTP2AkamaiFingerprint

	// Merge sources: JA3/JA4/JA3n/JA4O/HTTP2 from browserleaks, peetprint/
	// headers from peet.
	merged.Sources = Sources{
		JA3:       bl.Sources.JA3,
		JA3n:      bl.Sources.JA3n,
		JA4:       bl.Sources.JA4,
		JA4O:      bl.Sources.JA4O,
		Peetprint: peet.Sources.Peetprint,
		HTTP2:     bl.Sources.HTTP2,
		Headers:   peet.Sources.Headers,
	}
	return merged
}

func splitHeader(h string) (name, val string) {
	idx := strings.IndexByte(h, ':')
	if idx < 0 {
		return strings.TrimSpace(h), ""
	}
	return strings.ToLower(strings.TrimSpace(h[:idx])), strings.TrimSpace(h[idx+1:])
}

// FieldDiff is a single observed-vs-expected mismatch.
type FieldDiff struct {
	Field    string
	Expected string
	Observed string
}

// Compare reports the per-field differences between an Observed (go-stealth
// client) and a Reference (real browser). Fields the reference explicitly
// excludes (listed in Notes) are skipped with a nil diff and the reason
// returned in skipped.
//
// # Same-service invariant
//
// For each metric, Compare checks that the reference and observed Sources agree
// before comparing values. If they disagree, the metric is reported as a
// source-violation FieldDiff (field name prefixed "source:") and the values are
// NOT compared — a cross-service comparison reports a tooling artefact (e.g.
// peet's non-FoxIO JA4 vs browserleaks's FoxIO JA4) as a fingerprint defect.
//
// # sec-ch-ua normalization
//
// The GREASE brand (the brand that is neither "Chromium" nor "Google Chrome"/
// "Microsoft Edge") is seed-permuted by Chrome per connection, so a raw string
// compare flaps. NormalizeSecChUa replaces it with a placeholder before
// comparing. The Chromium and Google Chrome brands (with version) are compared
// exactly — those MUST match.
//
// # TLS fields
//
// ja3_hash, ja3n_hash, ja4, ja4_o, peetprint_hash, and the akamai HTTP/2
// fingerprint are compared directly: both peet and browserleaks strip RFC 8701
// GREASE from them before hashing, so they are stable across connections.
func Compare(o Observed, r *Reference) (diffs []FieldDiff, skipped []string) {
	skipUA := r.noteExcludes("user-agent")
	skipSecChUa := r.noteExcludes("sec-ch-ua")

	// --- JA3 (hash) ---
	if v := checkSource("ja3", r.Sources.JA3, o.Sources.JA3); v != nil {
		diffs = append(diffs, *v)
	} else if o.TLS.JA3Hash != r.TLS.JA3Hash {
		diffs = append(diffs, FieldDiff{"ja3_hash", r.TLS.JA3Hash, o.TLS.JA3Hash})
	}

	// --- JA3n (order-insensitive, browserleaks) ---
	if r.TLS.JA3nHash != "" || r.Sources.JA3n != "" {
		if v := checkSource("ja3n", r.Sources.JA3n, o.Sources.JA3n); v != nil {
			diffs = append(diffs, *v)
		} else if o.TLS.JA3nHash != r.TLS.JA3nHash {
			diffs = append(diffs, FieldDiff{"ja3n_hash", r.TLS.JA3nHash, o.TLS.JA3nHash})
		}
	}

	// --- JA4 (browserleaks, FoxIO-faithful) ---
	if v := checkSource("ja4", r.Sources.JA4, o.Sources.JA4); v != nil {
		diffs = append(diffs, *v)
	} else if o.TLS.JA4 != r.TLS.JA4 {
		diffs = append(diffs, FieldDiff{"ja4", r.TLS.JA4, o.TLS.JA4})
	}

	// --- JA4_o (original order, browserleaks) ---
	if r.TLS.JA4O != "" || r.Sources.JA4O != "" {
		if v := checkSource("ja4_o", r.Sources.JA4O, o.Sources.JA4O); v != nil {
			diffs = append(diffs, *v)
		} else if o.TLS.JA4O != r.TLS.JA4O {
			diffs = append(diffs, FieldDiff{"ja4_o", r.TLS.JA4O, o.TLS.JA4O})
		}
	}

	// --- peetprint (peet only) ---
	if r.TLS.PeetprintHash != "" || r.Sources.Peetprint != "" {
		if v := checkSource("peetprint", r.Sources.Peetprint, o.Sources.Peetprint); v != nil {
			diffs = append(diffs, *v)
		} else if o.TLS.PeetprintHash != r.TLS.PeetprintHash {
			diffs = append(diffs, FieldDiff{"peetprint_hash", r.TLS.PeetprintHash, o.TLS.PeetprintHash})
		}
	}

	// --- HTTP/2 Akamai fingerprint ---
	if v := checkSource("http2_akamai", r.Sources.HTTP2, o.Sources.HTTP2); v != nil {
		diffs = append(diffs, *v)
	} else if o.HTTP2AkamaiFingerprint != r.HTTP2AkamaiFingerprint {
		diffs = append(diffs, FieldDiff{"http2_akamai", r.HTTP2AkamaiFingerprint, o.HTTP2AkamaiFingerprint})
	}

	// --- header order + sec-ch-ua + accept* (peet only) ---
	if r.Sources.Headers != "" || o.Sources.Headers != "" {
		if v := checkSource("header_order", r.Sources.Headers, o.Sources.Headers); v != nil {
			diffs = append(diffs, *v)
		} else {
			if !equalStringSlices(o.HeaderOrder, r.HeaderOrder) {
				diffs = append(diffs, FieldDiff{
					Field:    "header_order",
					Expected: strings.Join(r.HeaderOrder, ","),
					Observed: strings.Join(o.HeaderOrder, ","),
				})
			}
			if !skipSecChUa {
				if NormalizeSecChUa(o.SecChUa) != NormalizeSecChUa(r.SecChUa) && r.SecChUa != "" {
					diffs = append(diffs, FieldDiff{"sec_ch_ua", r.SecChUa, o.SecChUa})
				}
			} else {
				skipped = append(skipped, "sec-ch-ua: excluded by reference note (headless mode)")
			}
			if o.SecChUaMobile != r.SecChUaMobile && r.SecChUaMobile != "" {
				diffs = append(diffs, FieldDiff{"sec_ch_ua_mobile", r.SecChUaMobile, o.SecChUaMobile})
			}
			if o.SecChUaPlatform != r.SecChUaPlatform && r.SecChUaPlatform != "" {
				diffs = append(diffs, FieldDiff{"sec_ch_ua_platform", r.SecChUaPlatform, o.SecChUaPlatform})
			}
			if o.Accept != r.Accept && r.Accept != "" {
				diffs = append(diffs, FieldDiff{"accept", r.Accept, o.Accept})
			}
			if o.AcceptLanguage != r.AcceptLanguage && r.AcceptLanguage != "" {
				diffs = append(diffs, FieldDiff{"accept_language", r.AcceptLanguage, o.AcceptLanguage})
			}
			if o.AcceptEncoding != r.AcceptEncoding && r.AcceptEncoding != "" {
				diffs = append(diffs, FieldDiff{"accept_encoding", r.AcceptEncoding, o.AcceptEncoding})
			}
		}
	}

	// --- user-agent ---
	if !skipUA {
		if o.UserAgent != r.UserAgent && r.UserAgent != "" {
			diffs = append(diffs, FieldDiff{"user_agent", r.UserAgent, o.UserAgent})
		}
	} else {
		skipped = append(skipped, "user-agent: excluded by reference note (headless mode)")
	}

	return diffs, skipped
}

// checkSource returns a non-nil FieldDiff if the reference and observed sources
// for a metric disagree. The FieldDiff's Field is prefixed "source:" so the
// caller can distinguish a same-service violation from a value mismatch. A
// cross-service comparison is a hard FAIL — the values are not compared.
func checkSource(metric, refSrc, obsSrc string) *FieldDiff {
	if refSrc == "" || obsSrc == "" {
		return nil // metric not present on one side; value comparison handles it
	}
	if refSrc == obsSrc {
		return nil
	}
	return &FieldDiff{
		Field:    "source:" + metric,
		Expected: fmt.Sprintf("reference source=%s", refSrc),
		Observed: fmt.Sprintf("observed source=%s — cross-service comparison is a tooling artefact, not a fingerprint defect", obsSrc),
	}
}

func (r *Reference) noteExcludes(field string) bool {
	for _, n := range r.Notes {
		if strings.Contains(n, "excludes "+field) || strings.Contains(n, field+": excluded") {
			return true
		}
	}
	return false
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// NormalizeSecChUa replaces the GREASE brand in a sec-ch-ua header value with
// the placeholder "GREASE". The GREASE brand is the brand token that is neither
// "Chromium" nor "Google Chrome" nor "Microsoft Edge" — Chrome seed-permutes it
// per connection, so a raw compare flaps. The remaining brands (with their
// versions) are compared exactly.
//
// Example:
//
//	`"Not_A Brand";v="24", "Chromium";v="146", "Google Chrome";v="146"`
//	-> `GREASE, "Chromium";v="146", "Google Chrome";v="146"`
func NormalizeSecChUa(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ", ")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		brand := strings.Trim(strings.SplitN(p, ";", 2)[0], `"`)
		if brand != "Chromium" && brand != "Google Chrome" && brand != "Microsoft Edge" {
			parts[i] = "GREASE"
		}
	}
	return strings.Join(parts, ", ")
}

// MD5Hex returns the hex md5 of s. Used by the capture tool to re-derive a
// normalized hash if needed; the oracle compares the services' reported hashes
// directly.
func MD5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
