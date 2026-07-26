package stealth

import (
	"errors"
	"fmt"
	"testing"

	"github.com/anatolykoptev/go-stealth/internal/uri"
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
		if got := uri.ExtractHost(tt.input); got != tt.want {
			t.Errorf("ExtractHost(%q) = %q, want %q", tt.input, got, tt.want)
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

// --- hard red tests ---

func TestCloudflareCookieMiddleware_NilHeaders_NoPanic(t *testing.T) {
	t.Parallel()

	provider := &mockCookieProvider{cookie: "cf_clearance=abc"}
	base := func(req *Request) (*Response, error) {
		return &Response{StatusCode: 200, Body: []byte("ok")}, nil
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	_, err := handler(&Request{Method: "GET", URL: "https://example.com", Headers: nil})

	if err != nil {
		t.Fatalf("unexpected error with nil headers: %v", err)
	}
}

func TestCloudflareCookieMiddleware_RetryCookie_NotStale(t *testing.T) {
	t.Parallel()

	provider := &mockCookieProvider{cookie: "cf_clearance=stale"}
	challengeBody := `<html><script src="/cdn-cgi/challenge-platform/x.js"></script></html>`

	var retryCookie string
	var callCount int
	base := func(req *Request) (*Response, error) {
		callCount++
		if callCount == 1 {
			return &Response{
				StatusCode: 503, Body: []byte(challengeBody),
				Headers: map[string]string{"server": "cloudflare"},
			}, nil
		}
		retryCookie = req.Headers["cookie"]
		return &Response{StatusCode: 200, Body: []byte("ok")}, nil
	}

	provider.solveFunc = func(domain string, _ *CloudflareError) (string, error) {
		return "cf_clearance=fresh", nil
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	_, err := handler(&Request{Method: "GET", URL: "https://example.com", Headers: map[string]string{}})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retryCookie != "cf_clearance=fresh" {
		t.Errorf("retry cookie = %q, want %q (must not contain stale cookie)", retryCookie, "cf_clearance=fresh")
	}
}

func TestCloudflareCookieMiddleware_SolveEmptyCookie_ReturnsError(t *testing.T) {
	t.Parallel()

	provider := &mockCookieProvider{
		solveFunc: func(domain string, _ *CloudflareError) (string, error) {
			return "", nil // no error but empty cookie
		},
	}
	challengeBody := `<html><script src="/cdn-cgi/challenge-platform/x.js"></script></html>`

	base := func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 503, Body: []byte(challengeBody),
			Headers: map[string]string{"server": "cloudflare"},
		}, nil
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	_, err := handler(&Request{Method: "GET", URL: "https://example.com", Headers: map[string]string{}})

	if err == nil {
		t.Fatal("expected error when solve returns empty cookie")
	}
}

func TestCloudflareCookieMiddleware_SolveError_UnwrappableBothErrors(t *testing.T) {
	t.Parallel()

	solveErr := fmt.Errorf("timeout connecting to solver")
	provider := &mockCookieProvider{
		solveFunc: func(domain string, _ *CloudflareError) (string, error) {
			return "", solveErr
		},
	}
	challengeBody := `<html><script src="/cdn-cgi/challenge-platform/x.js"></script></html>`

	base := func(req *Request) (*Response, error) {
		return &Response{
			StatusCode: 503, Body: []byte(challengeBody),
			Headers: map[string]string{"server": "cloudflare"},
		}, nil
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	_, err := handler(&Request{Method: "GET", URL: "https://example.com", Headers: map[string]string{}})

	if err == nil {
		t.Fatal("expected error")
	}
	// Must be able to unwrap both the CloudflareError and the solve error
	var cfErr *CloudflareError
	if !errors.As(err, &cfErr) {
		t.Errorf("errors.As(*CloudflareError) failed on %v", err)
	}
	if !errors.Is(err, solveErr) {
		t.Errorf("errors.Is(solveErr) failed — solve error lost in wrapping")
	}
}

func TestCloudflareCookieMiddleware_PropagatesBaseError(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("dns resolution failed")
	provider := &mockCookieProvider{}
	base := func(req *Request) (*Response, error) {
		return nil, baseErr
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	_, err := handler(&Request{Method: "GET", URL: "https://example.com", Headers: map[string]string{}})

	if !errors.Is(err, baseErr) {
		t.Errorf("expected base error to propagate, got %v", err)
	}
}

func TestCloudflareCookieMiddleware_ExistingCookiesPreserved(t *testing.T) {
	t.Parallel()

	provider := &mockCookieProvider{cookie: "cf_clearance=abc"}
	var capturedCookie string
	base := func(req *Request) (*Response, error) {
		capturedCookie = req.Headers["cookie"]
		return &Response{StatusCode: 200, Body: []byte("ok")}, nil
	}

	handler := CloudflareCookieMiddleware(provider)(base)
	_, err := handler(&Request{
		Method:  "GET",
		URL:     "https://example.com",
		Headers: map[string]string{"cookie": "session=xyz"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCookie != "session=xyz; cf_clearance=abc" {
		t.Errorf("cookie = %q, want existing cookies preserved with cf_clearance appended", capturedCookie)
	}
}

// TestCloudflareCookieMiddleware_RetriesOn403Challenge: solve-and-retry on a
// 403 challenge (cf-mitigated: challenge). Every existing cookie test uses 503;
// this pins the 403 path that was silently unexercised before the issue #48 fix.
func TestCloudflareCookieMiddleware_RetriesOn403Challenge(t *testing.T) {
	t.Parallel()

	provider := &mockCookieProvider{}
	var callCount int

	base := func(req *Request) (*Response, error) {
		callCount++
		if callCount == 1 {
			return &Response{
				StatusCode: 403,
				Body:       []byte("<html><title>Just a moment...</title></html>"),
				Headers:    map[string]string{"server": "cloudflare", "cf-mitigated": "challenge"},
			}, nil
		}
		return &Response{StatusCode: 200, Body: []byte("ok")}, nil
	}

	provider.solveFunc = func(domain string, challenge *CloudflareError) (string, error) {
		if challenge.Type != ChallengeJS {
			return "", fmt.Errorf("expected ChallengeJS at 403, got %q", challenge.Type)
		}
		return "cf_clearance=solved403", nil
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
		t.Errorf("expected 2 calls (403 challenge + retry), got %d", callCount)
	}
}
