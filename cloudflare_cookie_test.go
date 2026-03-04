package stealth

import (
	"fmt"
	"testing"
)

// --- mock for CookieProvider ---

type mockCookieProvider struct {
	cookie    string
	solveFunc func(domain string, challenge *CloudflareError) (string, error)
}

func (m *mockCookieProvider) GetCookie(domain string) string {
	return m.cookie
}

func (m *mockCookieProvider) Solve(domain string, challenge *CloudflareError) (string, error) {
	if m.solveFunc != nil {
		return m.solveFunc(domain, challenge)
	}
	return "", fmt.Errorf("no solver configured")
}

// --- tests ---

func TestCloudflareCookieMiddleware_InjectsCookie(t *testing.T) {
	t.Parallel()

	provider := &mockCookieProvider{cookie: "cf_clearance=abc123"}

	var capturedCookie string
	base := func(req *Request) (*Response, error) {
		capturedCookie = req.Headers["cookie"]
		return &Response{StatusCode: 200, Body: []byte("ok")}, nil
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	_, err := handler(&Request{
		Method:  "GET",
		URL:     "https://example.com/page",
		Headers: map[string]string{},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCookie != "cf_clearance=abc123" {
		t.Errorf("cookie = %q, want %q", capturedCookie, "cf_clearance=abc123")
	}
}

func TestCloudflareCookieMiddleware_RetriesOnChallenge(t *testing.T) {
	t.Parallel()

	provider := &mockCookieProvider{}
	var callCount int

	challengeBody := `<html><title>Just a moment...</title>
<script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js"></script></html>`

	base := func(req *Request) (*Response, error) {
		callCount++
		if callCount == 1 {
			return &Response{
				StatusCode: 503,
				Body:       []byte(challengeBody),
				Headers:    map[string]string{"server": "cloudflare"},
			}, nil
		}
		return &Response{StatusCode: 200, Body: []byte("ok")}, nil
	}

	provider.solveFunc = func(domain string, challenge *CloudflareError) (string, error) {
		return "cf_clearance=solved", nil
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	resp, err := handler(&Request{
		Method:  "GET",
		URL:     "https://example.com/page",
		Headers: map[string]string{},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (challenge + retry), got %d", callCount)
	}
}

func TestCloudflareCookieMiddleware_NoCookiePassthrough(t *testing.T) {
	t.Parallel()

	provider := &mockCookieProvider{}

	base := func(req *Request) (*Response, error) {
		return &Response{StatusCode: 200, Body: []byte("ok")}, nil
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	resp, err := handler(&Request{
		Method:  "GET",
		URL:     "https://example.com/page",
		Headers: map[string]string{},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestCloudflareCookieMiddleware_SolveFailsReturnsError(t *testing.T) {
	t.Parallel()

	provider := &mockCookieProvider{
		solveFunc: func(domain string, challenge *CloudflareError) (string, error) {
			return "", fmt.Errorf("solver unavailable")
		},
	}

	challengeBody := `<html><title>Just a moment...</title>
<script src="/cdn-cgi/challenge-platform/scripts/jsd/main.js"></script></html>`

	base := func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 503,
			Body:       []byte(challengeBody),
			Headers:    map[string]string{"server": "cloudflare"},
		}, nil
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	_, err := handler(&Request{
		Method:  "GET",
		URL:     "https://example.com/page",
		Headers: map[string]string{},
	})

	if err == nil {
		t.Fatal("expected error when solver fails")
	}
}

func TestExtractDomain(t *testing.T) {
	t.Parallel()

	tests := []struct{ input, want string }{
		{"https://example.com/page", "example.com"},
		{"http://sub.example.com:8080/path", "sub.example.com"},
		{"https://example.com", "example.com"},
		{"example.com", "example.com"},
	}
	for _, tt := range tests {
		if got := extractDomain(tt.input); got != tt.want {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAppendCookie(t *testing.T) {
	t.Parallel()

	if got := appendCookie("", "cf_clearance=abc"); got != "cf_clearance=abc" {
		t.Errorf("appendCookie empty = %q", got)
	}
	if got := appendCookie("session=xyz", "cf_clearance=abc"); got != "session=xyz; cf_clearance=abc" {
		t.Errorf("appendCookie existing = %q", got)
	}
}
