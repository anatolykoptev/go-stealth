package stealth

import "testing"

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
