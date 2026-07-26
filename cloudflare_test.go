package stealth

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsCloudflareChallenge_JsChallenge(t *testing.T) {
	t.Parallel()

	body := `<!DOCTYPE html><html><head><title>Just a moment...</title></head>
<body><div id="challenge-running">Please wait while we verify your browser...</div>
<script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js"></script></body></html>`

	resp := &Response{StatusCode: 503, Body: []byte(body), Headers: map[string]string{"server": "cloudflare"}}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError for JS challenge page")
	}
	if cfErr.Type != ChallengeJS {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeJS)
	}
}

func TestIsCloudflareChallenge_ManagedChallenge(t *testing.T) {
	t.Parallel()

	body := `<!DOCTYPE html><html><head><title>Just a moment...</title></head>
<body><div id="challenge-running">Verify you are human</div>
<div id="turnstile-wrapper"></div></body></html>`

	resp := &Response{StatusCode: 403, Body: []byte(body), Headers: map[string]string{"server": "cloudflare"}}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError for managed challenge")
	}
	if cfErr.Type != ChallengeTurnstile {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeTurnstile)
	}
}

func TestIsCloudflareChallenge_NotCloudflare(t *testing.T) {
	t.Parallel()

	resp := &Response{StatusCode: 503, Body: []byte("Service Unavailable"), Headers: map[string]string{"server": "nginx"}}
	cfErr := DetectCloudflare(resp)
	if cfErr != nil {
		t.Errorf("expected nil for non-Cloudflare 503, got %v", cfErr)
	}
}

func TestIsCloudflareChallenge_Normal200(t *testing.T) {
	t.Parallel()

	resp := &Response{StatusCode: 200, Body: []byte("<html>OK</html>"), Headers: map[string]string{"server": "cloudflare"}}
	cfErr := DetectCloudflare(resp)
	if cfErr != nil {
		t.Errorf("expected nil for 200 response, got %v", cfErr)
	}
}

func TestIsCloudflareChallenge_BlockedPage(t *testing.T) {
	t.Parallel()

	body := `<!DOCTYPE html><html><head><title>Attention Required! | Cloudflare</title></head>
<body><h1>Sorry, you have been blocked</h1><p>cf-error-details</p></body></html>`

	resp := &Response{StatusCode: 403, Body: []byte(body), Headers: map[string]string{"server": "cloudflare"}}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError for blocked page")
	}
	if cfErr.Type != ChallengeBlock {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeBlock)
	}
}

func TestCloudflareError_Error(t *testing.T) {
	t.Parallel()

	cfErr := &CloudflareError{Type: ChallengeJS, StatusCode: 503}
	s := cfErr.Error()
	if s == "" {
		t.Error("Error() returned empty string")
	}
}

func TestCloudflareDetectMiddleware_ReturnsErrorOnChallenge(t *testing.T) {
	t.Parallel()

	challengeBody := `<html><title>Just a moment...</title>
<script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js"></script></html>`

	base := func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 503,
			Body:       []byte(challengeBody),
			Headers:    map[string]string{"server": "cloudflare", "cf-ray": "abc123"},
		}, nil
	}

	handler := CloudflareDetectMiddleware(base)
	_, err := handler(&Request{Method: "GET", URL: "https://example.com"})

	if err == nil {
		t.Fatal("expected CloudflareError")
	}
	cfErr, ok := err.(*CloudflareError)
	if !ok {
		t.Fatalf("expected *CloudflareError, got %T", err)
	}
	if cfErr.Type != ChallengeJS {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeJS)
	}
	if cfErr.RayID != "abc123" {
		t.Errorf("RayID = %q, want %q", cfErr.RayID, "abc123")
	}
}

func TestCloudflareDetectMiddleware_PassthroughNormal(t *testing.T) {
	t.Parallel()

	base := func(req *Request) (*Response, error) {
		return &Response{StatusCode: 200, Body: []byte("ok"), Headers: map[string]string{"server": "nginx"}}, nil
	}

	handler := CloudflareDetectMiddleware(base)
	resp, err := handler(&Request{Method: "GET", URL: "https://example.com"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestCloudflareDetectMiddleware_Passthrough503NonCF(t *testing.T) {
	t.Parallel()

	base := func(req *Request) (*Response, error) {
		return &Response{StatusCode: 503, Body: []byte("down"), Headers: map[string]string{"server": "nginx"}}, nil
	}

	handler := CloudflareDetectMiddleware(base)
	resp, err := handler(&Request{Method: "GET", URL: "https://example.com"})

	if err != nil {
		t.Fatalf("unexpected error for non-CF 503: %v", err)
	}
	if resp.StatusCode != 503 {
		t.Errorf("StatusCode = %d, want 503", resp.StatusCode)
	}
}

// --- hard red tests ---

func TestDetectCloudflare_NilHeaders_NoPanic(t *testing.T) {
	t.Parallel()

	resp := &Response{StatusCode: 503, Body: []byte("challenge-platform"), Headers: nil}
	cfErr := DetectCloudflare(resp)
	if cfErr != nil {
		t.Errorf("expected nil for nil headers, got %v", cfErr)
	}
}

// TestDetectCloudflare_403WithChallengeplatform_IsJS: 403 + challenge-platform
// must return ChallengeJS. Cloudflare moved the JS/managed-challenge status from
// 503 to 403 on 2023-03-01; challenge-platform is a status-independent marker.
// (Was TestDetectCloudflare_403WithChallengeplatform_NotJS — encoded the 503-only
// bug as intent; flipped in the issue #48 fix.)
func TestDetectCloudflare_403WithChallengeplatform_IsJS(t *testing.T) {
	t.Parallel()

	body := `<html><script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js"></script></html>`
	resp := &Response{StatusCode: 403, Body: []byte(body), Headers: map[string]string{"server": "cloudflare"}}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("403 + challenge-platform should return ChallengeJS, got nil")
	}
	if cfErr.Type != ChallengeJS {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeJS)
	}
}

func TestDetectCloudflare_MixedCaseServer(t *testing.T) {
	t.Parallel()

	for _, server := range []string{"Cloudflare", "CLOUDFLARE", "cloudflare", "Cloudflare-nginx"} {
		body := `<html>you have been blocked</html>`
		resp := &Response{StatusCode: 403, Body: []byte(body), Headers: map[string]string{"server": server}}
		cfErr := DetectCloudflare(resp)
		if cfErr == nil {
			t.Errorf("server=%q: expected CloudflareError, got nil", server)
		}
	}
}

func TestDetectCloudflare_CF403NoMarkers_ReturnsNil(t *testing.T) {
	t.Parallel()

	// Cloudflare 403 but body has no challenge/block markers — normal CF 403
	resp := &Response{StatusCode: 403, Body: []byte("<html>Access denied</html>"), Headers: map[string]string{"server": "cloudflare"}}
	cfErr := DetectCloudflare(resp)
	if cfErr != nil {
		t.Errorf("CF 403 without markers should return nil, got %v", cfErr)
	}
}

func TestDetectCloudflare_RealRayID(t *testing.T) {
	t.Parallel()

	body := `<html>you have been blocked<span class="cf-error-details"></span></html>`
	resp := &Response{
		StatusCode: 403,
		Body:       []byte(body),
		Headers:    map[string]string{"server": "cloudflare", "cf-ray": "8f3a2b1c4d5e6-LAX"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError")
	}
	if cfErr.RayID != "8f3a2b1c4d5e6-LAX" {
		t.Errorf("RayID = %q, want %q", cfErr.RayID, "8f3a2b1c4d5e6-LAX")
	}
}

func TestCloudflareDetectMiddleware_PropagatesBaseError(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("connection refused")
	base := func(req *Request) (*Response, error) {
		return nil, baseErr
	}

	handler := CloudflareDetectMiddleware(base)
	_, err := handler(&Request{Method: "GET", URL: "https://example.com"})

	if !errors.Is(err, baseErr) {
		t.Errorf("expected base error to propagate, got %v", err)
	}
}

func TestCloudflareDetectMiddleware_ErrorsAs_CloudflareError(t *testing.T) {
	t.Parallel()

	body := `<html><script src="/cdn-cgi/challenge-platform/x.js"></script></html>`
	base := func(req *Request) (*Response, error) {
		return &Response{StatusCode: 503, Body: []byte(body), Headers: map[string]string{"server": "cloudflare"}}, nil
	}

	handler := CloudflareDetectMiddleware(base)
	_, err := handler(&Request{Method: "GET", URL: "https://example.com"})

	var cfErr *CloudflareError
	if !errors.As(err, &cfErr) {
		t.Fatalf("errors.As failed to extract *CloudflareError from %T: %v", err, err)
	}
	if cfErr.Type != ChallengeJS {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeJS)
	}
}

// --- issue #48: 403 managed-challenge detection (cf-mitigated, status-independent) ---

// TestDetectCloudflare_403_CfMitigated_ChallengePlatform: the exact issue #48
// case — 403 + cf-mitigated: challenge + challenge-platform + "Just a moment"
// title. Impossible to write green before the fix (503-only gate returned nil).
func TestDetectCloudflare_403_CfMitigated_ChallengePlatform(t *testing.T) {
	t.Parallel()

	body := `<!DOCTYPE html><html><head><title>Just a moment...</title></head>
<body><script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js"></script></body></html>`
	resp := &Response{
		StatusCode: 403,
		Body:       []byte(body),
		Headers:    map[string]string{"server": "cloudflare", "cf-mitigated": "challenge", "cf-ray": "8abc-LAX"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("403 + cf-mitigated:challenge + challenge-platform must be detected (issue #48), got nil")
	}
	if cfErr.Type != ChallengeJS {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeJS)
	}
	if cfErr.StatusCode != 403 {
		t.Errorf("StatusCode = %d, want 403", cfErr.StatusCode)
	}
}

// TestDetectCloudflare_403_CfMitigated_EmptyBody: cf-mitigated: challenge alone
// suffices per the docs — the header is set on all challenge-page types
// regardless of body content. Empty body must still be detected.
func TestDetectCloudflare_403_CfMitigated_EmptyBody(t *testing.T) {
	t.Parallel()

	resp := &Response{
		StatusCode: 403,
		Body:       []byte(""),
		Headers:    map[string]string{"server": "cloudflare", "cf-mitigated": "challenge"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("403 + cf-mitigated:challenge + empty body must be detected (header alone suffices), got nil")
	}
	if cfErr.Type != ChallengeJS {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeJS)
	}
}

// TestDetectCloudflare_403_CfMitigated_BlockMarkers_Win: a 403 carrying both
// cf-mitigated: challenge AND block markers must classify as a block — block
// markers take precedence over the header so a WAF block is never sent to a
// solver.
func TestDetectCloudflare_403_CfMitigated_BlockMarkers_Win(t *testing.T) {
	t.Parallel()

	body := `<html><h1>Sorry, you have been blocked</h1><span class="cf-error-details"></span></html>`
	resp := &Response{
		StatusCode: 403,
		Body:       []byte(body),
		Headers:    map[string]string{"server": "cloudflare", "cf-mitigated": "challenge"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected ChallengeBlock, got nil")
	}
	if cfErr.Type != ChallengeBlock {
		t.Errorf("Type = %q, want %q (block markers must win over cf-mitigated)", cfErr.Type, ChallengeBlock)
	}
}

// TestDetectCloudflare_503_CfMitigated: the header is also valid on the legacy
// 503 status. 503 + cf-mitigated: challenge must be detected (not gated out).
func TestDetectCloudflare_503_CfMitigated(t *testing.T) {
	t.Parallel()

	resp := &Response{
		StatusCode: 503,
		Body:       []byte(""),
		Headers:    map[string]string{"server": "cloudflare", "cf-mitigated": "challenge"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("503 + cf-mitigated:challenge must be detected (header on legacy status), got nil")
	}
	if cfErr.Type != ChallengeJS {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeJS)
	}
}
