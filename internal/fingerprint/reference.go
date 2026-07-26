// Package fingerprint holds the reference-capture and oracle infrastructure that
// measures whether go-stealth's emitted TLS/HTTP2 fingerprint matches a real
// browser's. It is the measurement layer for the "do we actually look like
// Chrome?" question — without it, profile staleness produces no failing signal.
//
// The reference files (testdata/reference_chrome_<major>.json) are produced by
// the cmd/fingerprint-capture tool, which downloads a Chrome-for-Testing build
// matching a given major, runs it against the peet.ws echo endpoint, and records
// the observed fingerprint with full provenance. The oracle test (build-tagged
// //go:build fingerprint, so never in preflight) builds a go-stealth client per
// Chrome profile, hits the same endpoint, and compares observed-vs-reference
// field by field.
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

// Endpoint is the echo service the oracle and capture tool both hit. peet.ws
// reflects the client's TLS, HTTP/2, and header fingerprint back as JSON.
const Endpoint = "https://tls.peet.ws/api/all"

// Reference is a captured browser fingerprint used as the oracle's ground truth.
// A reference without provenance is unfalsifiable later — every field below the
// blank line exists so a future reader can decide whether the reference is still
// trustworthy.
type Reference struct {
	Browser        string    `json:"browser"`         // "Chrome" (Chrome-for-Testing) or "Chromium"
	BrowserVersion string    `json:"browser_version"` // full version, e.g. "146.0.7633.0"
	Major          string    `json:"major"`           // major version, e.g. "146"
	CaptureTime    time.Time `json:"capture_time"`
	Endpoint       string    `json:"endpoint"`
	Mode           string    `json:"mode"`           // "headless" | "headful"
	Arch           string    `json:"arch"`           // runtime.GOARCH of the capture host
	BrowserSource  string    `json:"browser_source"` // "chrome-for-testing" | "system-chromium"

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

	// Notes records caveats that make a field non-comparable — e.g. headless
	// mode excludes UA/sec-ch-ua because headless Chrome reports
	// HeadlessChrome/<ver>, not Chrome/<ver>. The oracle reads Notes to decide
	// which fields to skip with an explicit reason rather than silently.
	Notes []string `json:"notes,omitempty"`
}

// TLSFingerprint holds the TLS-layer fingerprint fields peet.ws reports. peet.ws
// strips RFC 8701 GREASE (?A?A) values from ja3/ja3_hash/ja4/peetprint before
// hashing, so these fields are stable across connections of the same client and
// can be compared directly without further GREASE normalization.
type TLSFingerprint struct {
	JA3           string `json:"ja3"`
	JA3Hash       string `json:"ja3_hash"`
	JA4           string `json:"ja4"`
	Peetprint     string `json:"peetprint"`
	PeetprintHash string `json:"peetprint_hash"`
}

// Observed is the fingerprint extracted from a peet.ws response for a single
// request. It mirrors the comparable subset of Reference.
type Observed struct {
	TLS                    TLSFingerprint
	HTTP2AkamaiFingerprint string
	HeaderOrder            []string
	SecChUa                string
	SecChUaMobile          string
	SecChUaPlatform        string
	Accept                 string
	AcceptLanguage         string
	AcceptEncoding         string
	UserAgent              string
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

// ParsePeet parses a peet.ws /api/all JSON body into a raw map.
func ParsePeet(body []byte) (map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse peet.ws response: %w", err)
	}
	return raw, nil
}

// ExtractObserved pulls the comparable fingerprint fields out of a raw peet.ws
// response map. Header order is the list of regular (non-pseudo) header names
// in the order received in the HTTP/2 HEADERS frame, lowercased.
func ExtractObserved(raw map[string]any) (Observed, error) {
	var o Observed

	tlsMap, _ := raw["tls"].(map[string]any)
	o.TLS.JA3, _ = tlsMap["ja3"].(string)
	o.TLS.JA3Hash, _ = tlsMap["ja3_hash"].(string)
	o.TLS.JA4, _ = tlsMap["ja4"].(string)
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
	return o, nil
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
// sec-ch-ua: the GREASE brand (the brand that is neither "Chromium" nor
// "Google Chrome"/"Microsoft Edge") is seed-permuted by Chrome per connection,
// so a raw string compare flaps. NormalizeSecChUa replaces it with a placeholder
// before comparing. The Chromium and Google Chrome brands (with version) are
// compared exactly — those MUST match.
//
// TLS fields (ja3/ja3_hash/ja4/peetprint_hash) and the akamai HTTP/2 fingerprint
// are compared directly: peet.ws already strips RFC 8701 GREASE from them before
// hashing, so they are stable across connections.
func Compare(o Observed, r *Reference) (diffs []FieldDiff, skipped []string) {
	skipUA := r.noteExcludes("user-agent")
	skipSecChUa := r.noteExcludes("sec-ch-ua")

	if !skipUA {
		if o.UserAgent != r.UserAgent && r.UserAgent != "" {
			diffs = append(diffs, FieldDiff{"user_agent", r.UserAgent, o.UserAgent})
		}
	} else {
		skipped = append(skipped, "user-agent: excluded by reference note (headless mode)")
	}

	if o.TLS.JA3Hash != r.TLS.JA3Hash {
		diffs = append(diffs, FieldDiff{"ja3_hash", r.TLS.JA3Hash, o.TLS.JA3Hash})
	}
	if o.TLS.JA4 != r.TLS.JA4 {
		diffs = append(diffs, FieldDiff{"ja4", r.TLS.JA4, o.TLS.JA4})
	}
	if o.TLS.PeetprintHash != r.TLS.PeetprintHash {
		diffs = append(diffs, FieldDiff{"peetprint_hash", r.TLS.PeetprintHash, o.TLS.PeetprintHash})
	}
	if o.HTTP2AkamaiFingerprint != r.HTTP2AkamaiFingerprint {
		diffs = append(diffs, FieldDiff{"http2_akamai", r.HTTP2AkamaiFingerprint, o.HTTP2AkamaiFingerprint})
	}

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
	return diffs, skipped
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
// normalized hash if needed; the oracle compares peet.ws's reported hashes
// directly.
func MD5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
