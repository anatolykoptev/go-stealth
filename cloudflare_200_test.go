package stealth

import "testing"

func TestDetectCloudflare_200_CfMitigatedHeader(t *testing.T) {
	t.Parallel()

	resp := &Response{
		StatusCode: 200,
		Body:       []byte("<html>Just a moment...</html>"),
		Headers:    map[string]string{"server": "cloudflare", "cf-mitigated": "challenge"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError for cf-mitigated: challenge at 200")
	}
	if cfErr.Type != ChallengeManagedAt200 {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeManagedAt200)
	}
	if cfErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", cfErr.StatusCode)
	}
}

func TestDetectCloudflare_200_CfChlOpt(t *testing.T) {
	t.Parallel()

	body := `<html><script>window._cf_chl_opt={cType:"managed"}</script></html>`
	resp := &Response{
		StatusCode: 200,
		Body:       []byte(body),
		Headers:    map[string]string{"server": "cloudflare"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError for _cf_chl_opt at 200")
	}
	if cfErr.Type != ChallengeManagedAt200 {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeManagedAt200)
	}
}

func TestDetectCloudflare_200_ChallengePlatform(t *testing.T) {
	t.Parallel()

	body := `<html><script src="/cdn-cgi/challenge-platform/h/g/orchestrate/chl_page/v1"></script></html>`
	resp := &Response{
		StatusCode: 200,
		Body:       []byte(body),
		Headers:    map[string]string{"server": "cloudflare"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError for challenge-platform at 200")
	}
	if cfErr.Type != ChallengeManagedAt200 {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeManagedAt200)
	}
}

func TestDetectCloudflare_200_Turnstile(t *testing.T) {
	t.Parallel()

	body := `<html><div class="cf-turnstile" data-sitekey="x"></div></html>`
	resp := &Response{
		StatusCode: 200,
		Body:       []byte(body),
		Headers:    map[string]string{"server": "cloudflare"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CloudflareError for cf-turnstile at 200")
	}
	if cfErr.Type != ChallengeTurnstile {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeTurnstile)
	}
	if cfErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", cfErr.StatusCode)
	}
}

func TestDetectCloudflare_200_Clean_NoFalsePositive(t *testing.T) {
	t.Parallel()

	resp := &Response{
		StatusCode: 200,
		Body:       []byte("<html><body>Normal page content</body></html>"),
		Headers:    map[string]string{"server": "cloudflare"},
	}
	cfErr := DetectCloudflare(resp)
	if cfErr != nil {
		t.Errorf("clean 200 should not be detected as challenge, got %v", cfErr)
	}
}
