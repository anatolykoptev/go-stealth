package stealth

import (
	"strings"
	"testing"
)

// TestCF_403ChallengePlatform: 403 + "challenge-platform" in body.
// JS challenge requires 503 specifically — 403 must NOT return ChallengeJS.
// There is no turnstile/block marker either, so result must be nil.
func TestCF_403ChallengePlatform(t *testing.T) {
	t.Parallel()

	body := `<html><script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js"></script></html>`
	resp := &Response{
		StatusCode: 403,
		Body:       []byte(body),
		Headers:    map[string]string{"server": "cloudflare"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr != nil {
		t.Errorf("403 + challenge-platform should return nil (JS requires 503), got %v (type=%q)", cfErr, cfErr.Type)
	}
}

// TestCF_NonStandardStatus: redirect, not-found, and rate-limit with server:cloudflare
// — none of these statuses are handled by DetectCloudflare, so result must be nil.
func TestCF_NonStandardStatus(t *testing.T) {
	t.Parallel()

	for _, code := range []int{301, 404, 429} {
		resp := &Response{
			StatusCode: code,
			Body:       []byte("<html>page</html>"),
			Headers:    map[string]string{"server": "cloudflare"},
		}
		cfErr := DetectCloudflare(resp)
		if cfErr != nil {
			t.Errorf("status %d with server:cloudflare should return nil, got %v", code, cfErr)
		}
	}
}

// TestCF_ServerHeaderVariants: header value variations that are less common but valid.
// "cloudflare-nginx" (compound), "Cloudflare " (trailing space) — all must be detected.
// Note: "Cloudflare" / "CLOUDFLARE" / "Cloudflare-nginx" are already covered in
// cloudflare_test.go:TestDetectCloudflare_MixedCaseServer; these are the remaining variants.
func TestCF_ServerHeaderVariants(t *testing.T) {
	t.Parallel()

	// Body with an unambiguous block marker so we get a definitive detection.
	body := `<html>you have been blocked</html>`

	variants := []struct {
		server  string
		wantNil bool
	}{
		// Must detect — contains "cloudflare" substring after ToLower
		{"cloudflare-nginx", false},
		// Trailing space: "cloudflare " — Contains("cloudflare") still true after ToLower
		{"Cloudflare ", false},
		// Unrelated compound: must NOT detect
		{"nginx-cloudflare-proxy", false}, // contains "cloudflare" — detected
	}

	for _, v := range variants {
		resp := &Response{
			StatusCode: 403,
			Body:       []byte(body),
			Headers:    map[string]string{"server": v.server},
		}
		cfErr := DetectCloudflare(resp)
		if v.wantNil && cfErr != nil {
			t.Errorf("server=%q: expected nil, got %v", v.server, cfErr)
		}
		if !v.wantNil && cfErr == nil {
			t.Errorf("server=%q: expected CloudflareError, got nil", v.server)
		}
	}
}

// TestCF_200_MultipleMarkers: both cf-mitigated:challenge header and cf-turnstile in body
// — cf-mitigated is checked first in code, so ManagedAt200 must take priority over Turnstile.
func TestCF_200_MultipleMarkers(t *testing.T) {
	t.Parallel()

	body := `<html><div class="cf-turnstile" data-sitekey="abc"></div></html>`
	resp := &Response{
		StatusCode: 200,
		Body:       []byte(body),
		Headers: map[string]string{
			"server":       "cloudflare",
			"cf-mitigated": "challenge",
		},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError with cf-mitigated + cf-turnstile at 200")
	}
	if cfErr.Type != ChallengeManagedAt200 {
		t.Errorf("Type = %q, want %q (cf-mitigated must take priority)", cfErr.Type, ChallengeManagedAt200)
	}
}

// TestCF_200_EmptyBody: server:cloudflare, status 200, empty body — no CF markers
// in body and no special headers → must return nil (clean response).
func TestCF_200_EmptyBody(t *testing.T) {
	t.Parallel()

	resp := &Response{
		StatusCode: 200,
		Body:       []byte(""),
		Headers:    map[string]string{"server": "cloudflare"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr != nil {
		t.Errorf("200 empty body with cloudflare server should return nil, got %v", cfErr)
	}
}

// TestCF_NilHeaders_At200: Response with nil Headers at status 200 — different code
// path from the 503 nil-headers test in cloudflare_test.go. Must not panic.
func TestCF_NilHeaders_At200(t *testing.T) {
	t.Parallel()

	resp := &Response{
		StatusCode: 200,
		Body:       []byte("<html>Just a moment...</html>"),
		Headers:    nil,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic with nil Headers at status 200: %v", r)
		}
	}()

	cfErr := DetectCloudflare(resp)
	// nil Headers → server = "" → no "cloudflare" → nil
	if cfErr != nil {
		t.Errorf("nil Headers at 200 should return nil, got %v", cfErr)
	}
}

// TestCF_LargeBody: 1 MB body with CF markers placed at the very end.
// strings.Contains must still find them (no early-exit truncation).
func TestCF_LargeBody(t *testing.T) {
	t.Parallel()

	const mbSize = 1 << 20 // 1 MB
	padding := strings.Repeat("A", mbSize)
	body := padding + `<script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js"></script>`

	resp := &Response{
		StatusCode: 503,
		Body:       []byte(body),
		Headers:    map[string]string{"server": "cloudflare"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError with CF marker at end of 1 MB body")
	}
	if cfErr.Type != ChallengeJS {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeJS)
	}
}

// TestCF_CfMitigated_NotChallenge: cf-mitigated header with value "yes" (not "challenge")
// — the code checks Contains("challenge"), so "yes" must NOT trigger ManagedAt200.
func TestCF_CfMitigated_NotChallenge(t *testing.T) {
	t.Parallel()

	resp := &Response{
		StatusCode: 200,
		Body:       []byte("<html>Welcome</html>"),
		Headers: map[string]string{
			"server":       "cloudflare",
			"cf-mitigated": "yes",
		},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr != nil {
		t.Errorf("cf-mitigated:yes should return nil (only 'challenge' triggers), got %v", cfErr)
	}
}

// TestCF_extractDomain_EdgeCases: boundary inputs for extractDomain.
// "example.com" (no scheme), URL with port, malformed "://weird", empty string.
func TestCF_extractDomain_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		// No scheme → no "://" → return rawURL as-is
		{"example.com", "example.com"},
		// Port in URL → strip at ':'
		{"https://example.com:8080/path/to/page", "example.com"},
		// Malformed — "://" found but host is "weird"
		{"://weird", "weird"},
		// Empty string → no "://" → return ""
		{"", ""},
		// Path separator after host → strip at '/'
		{"https://host.example.org/path", "host.example.org"},
		// Port only, no path
		{"http://api.example.com:9000", "api.example.com"},
	}

	for _, tt := range tests {
		got := extractDomain(tt.input)
		if got != tt.want {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestCF_appendCookie_EdgeCases: boundary inputs for appendCookie.
// Empty existing, single existing, multi-value existing.
func TestCF_appendCookie_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		existing string
		newCookie string
		want      string
	}{
		// Empty existing → return newCookie verbatim (no leading "; ")
		{"", "a=b", "a=b"},
		// Single existing → append with "; " separator
		{"x=y", "a=b", "x=y; a=b"},
		// Multi-value existing → append after last value
		{"x=y; z=w", "a=b", "x=y; z=w; a=b"},
		// New cookie is empty string → still appends (caller's responsibility)
		{"x=y", "", "x=y; "},
		// Both empty → returns ""
		{"", "", ""},
	}

	for _, tt := range tests {
		got := appendCookie(tt.existing, tt.newCookie)
		if got != tt.want {
			t.Errorf("appendCookie(%q, %q) = %q, want %q", tt.existing, tt.newCookie, got, tt.want)
		}
	}
}
