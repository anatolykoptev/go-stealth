# Cloudflare Detection & Cookie Middleware Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Cloudflare challenge detection middleware and a pluggable cookie provider middleware to go-stealth, enabling automatic detection of Cloudflare blocks and cookie injection from external solvers.

**Architecture:** Two composable middlewares: (1) `CloudflareDetectMiddleware` inspects responses for Cloudflare challenge pages (503 + challenge HTML markers) and returns a structured `*CloudflareError` with challenge type and metadata; (2) `CloudflareCookieMiddleware` wraps a `CookieProvider` interface — before each request it checks for cached `cf_clearance` cookies, and after a Cloudflare error it calls the provider to obtain a fresh cookie and retries. External solvers (ox-browser, headless Chrome, manual) implement the `CookieProvider` interface.

**Tech Stack:** Go 1.26, `net/http`, `strings`, `regexp`. No new external dependencies. Fits existing `Middleware` pattern from `middleware.go`.

---

### Task 1: CloudflareError type and detection function

**Files:**
- Create: `cloudflare.go`
- Create: `cloudflare_test.go`

**Step 1: Write the failing test**

```go
// cloudflare_test.go
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
```

**Step 2: Run test to verify it fails**

Run: `cd ~/src/go-stealth && go test -run TestIsCloudflareChallenge -v`
Expected: FAIL — types not defined.

**Step 3: Write implementation**

```go
// cloudflare.go
package stealth

import (
	"fmt"
	"strings"
)

// ChallengeType identifies the kind of Cloudflare challenge.
type ChallengeType string

const (
	ChallengeJS        ChallengeType = "js_challenge"       // 503 + JS computation
	ChallengeTurnstile ChallengeType = "managed_challenge"  // Turnstile CAPTCHA
	ChallengeBlock     ChallengeType = "block"              // IP/country block
)

// CloudflareError indicates a Cloudflare challenge or block was detected.
type CloudflareError struct {
	Type       ChallengeType
	StatusCode int
	RayID      string // CF-Ray header if present
}

func (e *CloudflareError) Error() string {
	return fmt.Sprintf("cloudflare %s (HTTP %d, ray %s)", e.Type, e.StatusCode, e.RayID)
}

// DetectCloudflare inspects a Response for Cloudflare challenge markers.
// Returns nil if the response is not a Cloudflare challenge.
func DetectCloudflare(resp *Response) *CloudflareError {
	if resp.StatusCode != 403 && resp.StatusCode != 503 {
		return nil
	}
	server := strings.ToLower(resp.Headers["server"])
	if !strings.Contains(server, "cloudflare") {
		return nil
	}

	body := strings.ToLower(string(resp.Body))
	rayID := resp.Headers["cf-ray"]

	// JS challenge: 503 + challenge-platform scripts
	if resp.StatusCode == 503 && strings.Contains(body, "challenge-platform") {
		return &CloudflareError{Type: ChallengeJS, StatusCode: resp.StatusCode, RayID: rayID}
	}

	// Turnstile managed challenge
	if strings.Contains(body, "turnstile-wrapper") || strings.Contains(body, "cf-turnstile") {
		return &CloudflareError{Type: ChallengeTurnstile, StatusCode: resp.StatusCode, RayID: rayID}
	}

	// Block page
	if strings.Contains(body, "you have been blocked") || strings.Contains(body, "cf-error-details") {
		return &CloudflareError{Type: ChallengeBlock, StatusCode: resp.StatusCode, RayID: rayID}
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd ~/src/go-stealth && go test -run "TestIsCloudflareChallenge|TestCloudflareError" -v`
Expected: PASS

**Step 5: Commit**

```bash
cd ~/src/go-stealth
git add cloudflare.go cloudflare_test.go
git commit -m "feat: add Cloudflare challenge detection (CloudflareError + DetectCloudflare)"
```

---

### Task 2: CloudflareDetectMiddleware

**Files:**
- Modify: `cloudflare.go`
- Modify: `cloudflare_test.go`

**Step 1: Write the failing test**

```go
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
```

**Step 2: Run test to verify it fails**

Run: `cd ~/src/go-stealth && go test -run TestCloudflareDetectMiddleware -v`
Expected: FAIL — `CloudflareDetectMiddleware` not defined.

**Step 3: Write implementation**

Add to `cloudflare.go`:

```go
// CloudflareDetectMiddleware inspects responses for Cloudflare challenges.
// If a challenge is detected, it returns a *CloudflareError (use errors.As to extract).
// Non-challenge responses pass through unchanged.
func CloudflareDetectMiddleware(next Handler) Handler {
	return func(req *Request) (*Response, error) {
		resp, err := next(req)
		if err != nil {
			return resp, err
		}
		if cfErr := DetectCloudflare(resp); cfErr != nil {
			return resp, cfErr
		}
		return resp, nil
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd ~/src/go-stealth && go test -run TestCloudflareDetectMiddleware -v`
Expected: PASS

**Step 5: Commit**

```bash
cd ~/src/go-stealth
git add cloudflare.go cloudflare_test.go
git commit -m "feat: add CloudflareDetectMiddleware"
```

---

### Task 3: CookieProvider interface and CloudflareCookieMiddleware

**Files:**
- Modify: `cloudflare.go`
- Modify: `cloudflare_test.go`

**Step 1: Write the failing tests**

```go
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

	// After challenge, provider returns a cookie
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

	provider := &mockCookieProvider{} // no cookie, no solver

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

// --- test helpers ---

type mockCookieProvider struct {
	cookie    string // pre-cached cookie
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
```

**Step 2: Run test to verify it fails**

Run: `cd ~/src/go-stealth && go test -run TestCloudflareCookieMiddleware -v`
Expected: FAIL — `CookieProvider`, `CloudflareCookieMiddleware` not defined.

**Step 3: Write implementation**

Add to `cloudflare.go`:

```go
// CookieProvider obtains Cloudflare clearance cookies from an external source.
// Implementations might use headless browsers, anti-captcha APIs, Redis cache, etc.
type CookieProvider interface {
	// GetCookie returns a cached cf_clearance cookie for the domain.
	// Returns empty string if no cached cookie is available.
	GetCookie(domain string) string

	// Solve attempts to solve a Cloudflare challenge and returns the cookie string.
	// The cookie string should be in "cf_clearance=value" format.
	// Called when a challenge is detected and no cached cookie exists.
	Solve(domain string, challenge *CloudflareError) (string, error)
}

// CloudflareCookieMiddleware returns a middleware that:
//  1. Injects cached cf_clearance cookies from the provider before each request.
//  2. On Cloudflare challenge response, calls provider.Solve() to get a cookie and retries once.
func CloudflareCookieMiddleware(provider CookieProvider) Middleware {
	return func(next Handler) Handler {
		return func(req *Request) (*Response, error) {
			domain := extractDomain(req.URL)

			// Inject cached cookie if available.
			if cookie := provider.GetCookie(domain); cookie != "" {
				if req.Headers == nil {
					req.Headers = make(map[string]string)
				}
				req.Headers["cookie"] = appendCookie(req.Headers["cookie"], cookie)
			}

			resp, err := next(req)
			if err != nil {
				return resp, err
			}

			// Check for Cloudflare challenge.
			cfErr := DetectCloudflare(resp)
			if cfErr == nil {
				return resp, nil
			}

			// Try to solve.
			cookie, solveErr := provider.Solve(domain, cfErr)
			if solveErr != nil {
				return resp, cfErr
			}
			if cookie == "" {
				return resp, cfErr
			}

			// Retry with the new cookie.
			if req.Headers == nil {
				req.Headers = make(map[string]string)
			}
			req.Headers["cookie"] = appendCookie(req.Headers["cookie"], cookie)
			return next(req)
		}
	}
}

// extractDomain extracts the hostname from a URL string.
func extractDomain(rawURL string) string {
	// Fast path: find "://" then extract host until "/" or ":"
	idx := strings.Index(rawURL, "://")
	if idx < 0 {
		return rawURL
	}
	host := rawURL[idx+3:]
	if i := strings.IndexAny(host, ":/"); i >= 0 {
		host = host[:i]
	}
	return host
}

// appendCookie appends a cookie to an existing cookie header value.
func appendCookie(existing, newCookie string) string {
	if existing == "" {
		return newCookie
	}
	return existing + "; " + newCookie
}
```

**Step 4: Run test to verify it passes**

Run: `cd ~/src/go-stealth && go test -run TestCloudflareCookieMiddleware -v`
Expected: PASS

**Step 5: Commit**

```bash
cd ~/src/go-stealth
git add cloudflare.go cloudflare_test.go
git commit -m "feat: add CookieProvider interface and CloudflareCookieMiddleware"
```

---

### Task 4: Helper tests (extractDomain, appendCookie)

**Files:**
- Modify: `cloudflare_test.go`

**Step 1: Write tests**

```go
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
```

**Step 2: Run tests**

Run: `cd ~/src/go-stealth && go test -run "TestExtractDomain|TestAppendCookie" -v`
Expected: PASS (these test already-implemented functions)

**Step 3: Commit**

```bash
cd ~/src/go-stealth
git add cloudflare_test.go
git commit -m "test: add helper tests for extractDomain and appendCookie"
```

---

### Task 5: Full test suite + lint

**Step 1: Run all Cloudflare tests**

Run: `cd ~/src/go-stealth && go test -run "TestCloudflare|TestIsCloudflare|TestExtractDomain|TestAppendCookie" -v -count=1`
Expected: ALL PASS

**Step 2: Run full test suite**

Run: `cd ~/src/go-stealth && go test ./... -count=1`
Expected: ALL PASS

**Step 3: Run vet + lint**

Run: `cd ~/src/go-stealth && go vet ./...`
Expected: clean

**Step 4: Fix any issues, commit if needed**

```bash
cd ~/src/go-stealth
git add -A
git commit -m "fix: address lint issues in cloudflare middleware"
```

---

## Usage Examples (for docs/README, not part of implementation)

### Detection only
```go
client, _ := stealth.NewClient(stealth.WithProfile(stealth.RandomProfile()))
client.Use(stealth.CloudflareDetectMiddleware)

_, _, _, err := client.Do("GET", "https://protected-site.com", nil, nil)
var cfErr *stealth.CloudflareError
if errors.As(err, &cfErr) {
    log.Printf("Cloudflare %s detected (ray: %s)", cfErr.Type, cfErr.RayID)
}
```

### With cookie provider
```go
solver := myapp.NewRedisCookieProvider(redisClient)  // implements stealth.CookieProvider
client.Use(stealth.CloudflareCookieMiddleware(solver))

// Middleware auto-injects cached cookies and retries on challenge
body, _, status, _ := client.Do("GET", "https://protected-site.com", nil, nil)
```
