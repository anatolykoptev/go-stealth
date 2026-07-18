# go-stealth + ox-browser Integration Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Integrate ox-browser's CF solving, enhanced detection, and web intelligence into go-stealth's middleware chain.

**Architecture:** 5 new files in go-stealth root package. `OxBrowserSolver` implements existing `CookieProvider` interface — drop-in replacement for `ByparrSolver`. `SmartFetchMiddleware` delegates CF-blocked requests to ox-browser `/fetch-smart`. Enhanced `DetectCloudflare` adds HTTP 200 challenge detection (ported from ox-browser Rust). `SiteIntel` provides per-domain tech/WAF caching via `/analyze`. All integration goes through a single `OxBrowserClient` HTTP wrapper.

**Tech Stack:** Go 1.26, go-stealth middleware system, ox-browser REST API (`:8901`)

**Repos:**
- `go-stealth`: `/home/krolik/src/go-stealth/` (all new code here)
- `ox-browser`: `/home/krolik/src/ox-browser/` (read-only reference for API contracts)

---

### Task 1: OxBrowser HTTP Client Wrapper

Thin HTTP client for calling ox-browser API. All subsequent tasks depend on this.

**Files:**
- Create: `oxbrowser.go`
- Create: `oxbrowser_test.go`

**Step 1: Write the failing test**

```go
// oxbrowser_test.go
package stealth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOxBrowserClient_Solve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/solve" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "test123"},
		})
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	cookies, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err != nil {
		t.Fatal(err)
	}
	if cookies["cf_clearance"] != "test123" {
		t.Errorf("expected cf_clearance=test123, got %v", cookies)
	}
}

func TestOxBrowserClient_FetchSmart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      200,
			"body":        "<html>ok</html>",
			"method":      "direct",
			"cf_detected": false,
			"elapsed_ms":  50,
		})
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	resp, err := client.FetchSmart(context.Background(), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 || resp.Method != "direct" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestOxBrowserClient_Analyze(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"url":    "https://example.com",
			"status": 200,
			"technologies": []map[string]interface{}{
				{"name": "Cloudflare", "categories": []string{"CDN"}, "confidence": 100},
			},
		})
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	resp, err := client.Analyze(context.Background(), "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Technologies) != 1 || resp.Technologies[0].Name != "Cloudflare" {
		t.Errorf("unexpected techs: %+v", resp.Technologies)
	}
}

func TestOxBrowserClient_SolveError(t *testing.T) {
	client := NewOxBrowserClient("http://127.0.0.1:1") // connection refused
	_, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/krolik/src/go-stealth && go test -run TestOxBrowser -v`
Expected: FAIL — `NewOxBrowserClient` undefined

**Step 3: Write minimal implementation**

```go
// oxbrowser.go
package stealth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OxBrowserClient calls the ox-browser REST API.
type OxBrowserClient struct {
	baseURL string
	client  *http.Client
}

// NewOxBrowserClient creates a client for ox-browser at the given base URL.
func NewOxBrowserClient(baseURL string) *OxBrowserClient {
	return &OxBrowserClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// SolveResponse is the response from /solve.
type SolveResponse struct {
	Status  string            `json:"status"`
	Cookies map[string]string `json:"cookies"`
	Error   string            `json:"error,omitempty"`
}

// FetchSmartResponse is the response from /fetch-smart.
type FetchSmartResponse struct {
	Status     int    `json:"status"`
	Body       string `json:"body"`
	Method     string `json:"method"`
	CfDetected bool   `json:"cf_detected"`
	ElapsedMs  int64  `json:"elapsed_ms"`
	Error      string `json:"error,omitempty"`
}

// AnalyzeTech is a single detected technology.
type AnalyzeTech struct {
	Name       string   `json:"name"`
	Categories []string `json:"categories"`
	Confidence int      `json:"confidence"`
	Version    *string  `json:"version,omitempty"`
}

// AnalyzeResponse is the response from /analyze.
type AnalyzeResponse struct {
	URL          string        `json:"url"`
	Status       int           `json:"status"`
	Technologies []AnalyzeTech `json:"technologies"`
	Error        string        `json:"error,omitempty"`
}

// Solve calls ox-browser /solve to get CF clearance cookies.
func (c *OxBrowserClient) Solve(ctx context.Context, url, challengeType string) (map[string]string, error) {
	body, _ := json.Marshal(map[string]string{"url": url, "challenge_type": challengeType})
	var result SolveResponse
	if err := c.post(ctx, "/solve", body, &result); err != nil {
		return nil, err
	}
	if result.Status != "ok" {
		return nil, fmt.Errorf("ox-browser solve: %s", result.Error)
	}
	return result.Cookies, nil
}

// FetchSmart calls ox-browser /fetch-smart (auto CF bypass).
func (c *OxBrowserClient) FetchSmart(ctx context.Context, url string) (*FetchSmartResponse, error) {
	body, _ := json.Marshal(map[string]string{"url": url})
	var result FetchSmartResponse
	if err := c.post(ctx, "/fetch-smart", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Analyze calls ox-browser /analyze for tech detection.
func (c *OxBrowserClient) Analyze(ctx context.Context, url string) (*AnalyzeResponse, error) {
	body, _ := json.Marshal(map[string]string{"url": url})
	var result AnalyzeResponse
	if err := c.post(ctx, "/analyze", body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *OxBrowserClient) post(ctx context.Context, path string, body []byte, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ox-browser: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("ox-browser %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ox-browser: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ox-browser %s: HTTP %d: %s", path, resp.StatusCode, string(respBody))
	}
	return json.Unmarshal(respBody, out)
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/krolik/src/go-stealth && go test -run TestOxBrowser -v`
Expected: PASS (4 tests)

**Step 5: Commit**

```bash
cd /home/krolik/src/go-stealth
git add oxbrowser.go oxbrowser_test.go
git commit -m "feat: add OxBrowserClient HTTP wrapper for ox-browser API"
```

---

### Task 2: OxBrowserSolver (CookieProvider implementation)

Drop-in replacement for `ByparrSolver`. Uses ox-browser `/solve` instead of Byparr.

**Dependencies:** Task 1

**Files:**
- Create: `solver_oxbrowser.go`
- Create: `solver_oxbrowser_test.go`

**Step 1: Write the failing test**

```go
// solver_oxbrowser_test.go
package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOxBrowserSolver_GetCookie_Empty(t *testing.T) {
	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: "http://unused"})
	if got := solver.GetCookie("example.com"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestOxBrowserSolver_Solve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "abc123"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL})
	challenge := &CloudflareError{Type: ChallengeJS, StatusCode: 503}
	cookie, err := solver.Solve("example.com", challenge)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "cf_clearance=abc123" {
		t.Errorf("expected cf_clearance=abc123, got %q", cookie)
	}

	// Should be cached now.
	if got := solver.GetCookie("example.com"); got != "cf_clearance=abc123" {
		t.Errorf("cache miss: got %q", got)
	}
}

func TestOxBrowserSolver_BlockNotSolvable(t *testing.T) {
	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: "http://unused"})
	challenge := &CloudflareError{Type: ChallengeBlock, StatusCode: 403}
	_, err := solver.Solve("example.com", challenge)
	if err == nil {
		t.Error("expected error for block challenge")
	}
}

func TestOxBrowserSolver_CacheTTL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "fresh"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{
		BaseURL:  srv.URL,
		CacheTTL: 1, // 1 nanosecond — expires immediately
	})
	challenge := &CloudflareError{Type: ChallengeJS, StatusCode: 503}
	_, err := solver.Solve("example.com", challenge)
	if err != nil {
		t.Fatal(err)
	}
	// Cache should have expired.
	if got := solver.GetCookie("example.com"); got != "" {
		t.Errorf("expected expired cache, got %q", got)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/krolik/src/go-stealth && go test -run TestOxBrowserSolver -v`
Expected: FAIL — `NewOxBrowserSolver` undefined

**Step 3: Write minimal implementation**

```go
// solver_oxbrowser.go
package stealth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// OxBrowserSolverConfig configures the ox-browser CF solver.
type OxBrowserSolverConfig struct {
	// BaseURL of ox-browser (e.g. "http://127.0.0.1:8901").
	BaseURL string

	// CacheTTL for solved cookies. Default: 25 minutes.
	CacheTTL time.Duration
}

// OxBrowserSolver implements CookieProvider using ox-browser /solve.
// Drop-in replacement for ByparrSolver.
type OxBrowserSolver struct {
	client *OxBrowserClient
	ttl    time.Duration

	mu    sync.RWMutex
	cache map[string]cachedCookie
}

// NewOxBrowserSolver creates a CookieProvider backed by ox-browser.
func NewOxBrowserSolver(cfg OxBrowserSolverConfig) *OxBrowserSolver {
	ttl := cfg.CacheTTL
	if ttl == 0 {
		ttl = 25 * time.Minute
	}
	return &OxBrowserSolver{
		client: NewOxBrowserClient(cfg.BaseURL),
		ttl:    ttl,
		cache:  make(map[string]cachedCookie),
	}
}

// GetCookie returns a cached cf_clearance cookie for the domain.
func (s *OxBrowserSolver) GetCookie(domain string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.cache[domain]
	if !ok || time.Now().After(entry.expiresAt) {
		return ""
	}
	return entry.cookie
}

// Solve calls ox-browser /solve to obtain CF clearance cookies.
func (s *OxBrowserSolver) Solve(domain string, challenge *CloudflareError) (string, error) {
	if challenge != nil && challenge.Type == ChallengeBlock {
		return "", fmt.Errorf("block challenges are not solvable")
	}

	challengeType := "js_challenge"
	if challenge != nil {
		challengeType = string(challenge.Type)
	}

	url := fmt.Sprintf("https://%s", domain)
	cookies, err := s.client.Solve(context.Background(), url, challengeType)
	if err != nil {
		return "", err
	}

	clearance, ok := cookies["cf_clearance"]
	if !ok {
		return "", fmt.Errorf("cf_clearance not found in ox-browser response")
	}

	cookie := fmt.Sprintf("cf_clearance=%s", clearance)

	s.mu.Lock()
	s.cache[domain] = cachedCookie{
		cookie:    cookie,
		expiresAt: time.Now().Add(s.ttl),
	}
	s.mu.Unlock()

	return cookie, nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/krolik/src/go-stealth && go test -run TestOxBrowserSolver -v`
Expected: PASS (4 tests)

**Step 5: Commit**

```bash
cd /home/krolik/src/go-stealth
git add solver_oxbrowser.go solver_oxbrowser_test.go
git commit -m "feat: add OxBrowserSolver — CookieProvider via ox-browser /solve"
```

---

### Task 3: Enhanced CF Detection (HTTP 200 challenges)

Port ox-browser's detection logic for `cf-mitigated` header, `_cf_chl_opt`, and `challenge-platform` at HTTP 200.

**Dependencies:** None (modifies existing code)

**Files:**
- Modify: `cloudflare.go` (add `ChallengeManagedAt200` type + HTTP 200 detection)
- Modify: existing tests in `cloudflare_test.go` or add inline

**Step 1: Write the failing tests**

Add to existing test file or create new:

```go
// cloudflare_200_test.go
package stealth

import "testing"

func TestDetectCloudflare_ManagedAt200_CfMitigated(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Headers: map[string]string{
			"server":       "cloudflare",
			"cf-mitigated": "challenge",
			"cf-ray":       "abc-LAX",
		},
		Body: []byte("<html>Just a moment...</html>"),
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CF detection at 200")
	}
	if cfErr.Type != ChallengeManagedAt200 {
		t.Errorf("expected managed_challenge_200, got %s", cfErr.Type)
	}
}

func TestDetectCloudflare_ManagedAt200_CfChlOpt(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Headers:    map[string]string{"server": "cloudflare"},
		Body:       []byte("<html><script>window._cf_chl_opt={}</script></html>"),
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CF detection for _cf_chl_opt")
	}
	if cfErr.Type != ChallengeManagedAt200 {
		t.Errorf("expected managed_challenge_200, got %s", cfErr.Type)
	}
}

func TestDetectCloudflare_ManagedAt200_ChallengePlatform(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Headers:    map[string]string{"server": "cloudflare"},
		Body:       []byte(`<script src="/cdn-cgi/challenge-platform/h/g/orchestrate"></script>`),
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected CF detection for challenge-platform at 200")
	}
}

func TestDetectCloudflare_TurnstileAt200(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Headers:    map[string]string{"server": "cloudflare"},
		Body:       []byte(`<div class="cf-turnstile"></div>`),
	}
	cfErr := DetectCloudflare(resp)
	if cfErr == nil {
		t.Fatal("expected turnstile detection at 200")
	}
	if cfErr.Type != ChallengeTurnstile {
		t.Errorf("expected turnstile, got %s", cfErr.Type)
	}
}

func TestDetectCloudflare_Clean200_NoFalsePositive(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Headers:    map[string]string{"server": "cloudflare"},
		Body:       []byte("<html><body>Normal page</body></html>"),
	}
	if cfErr := DetectCloudflare(resp); cfErr != nil {
		t.Errorf("false positive: %v", cfErr)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/krolik/src/go-stealth && go test -run TestDetectCloudflare_ -v`
Expected: FAIL — `ChallengeManagedAt200` undefined, HTTP 200 detection not triggered

**Step 3: Modify `cloudflare.go`**

Add new challenge type and extend `DetectCloudflare`:

```go
// Add to constants:
ChallengeManagedAt200 ChallengeType = "managed_challenge_200"

// Replace DetectCloudflare function:
func DetectCloudflare(resp *Response) *CloudflareError {
	server := strings.ToLower(resp.Headers["server"])
	if !strings.Contains(server, "cloudflare") {
		return nil
	}

	body := strings.ToLower(string(resp.Body))
	rayID := resp.Headers["cf-ray"]

	// --- HTTP 403/503 detection ---
	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		if resp.StatusCode == 503 && strings.Contains(body, "challenge-platform") {
			return &CloudflareError{Type: ChallengeJS, StatusCode: resp.StatusCode, RayID: rayID}
		}
		if strings.Contains(body, "turnstile-wrapper") || strings.Contains(body, "cf-turnstile") {
			return &CloudflareError{Type: ChallengeTurnstile, StatusCode: resp.StatusCode, RayID: rayID}
		}
		if strings.Contains(body, "you have been blocked") || strings.Contains(body, "cf-error-details") {
			return &CloudflareError{Type: ChallengeBlock, StatusCode: resp.StatusCode, RayID: rayID}
		}
		return nil
	}

	// --- HTTP 200 detection (interstitial challenges) ---
	if resp.StatusCode == 200 {
		cfMitigated := strings.ToLower(resp.Headers["cf-mitigated"])
		if strings.Contains(cfMitigated, "challenge") {
			return &CloudflareError{Type: ChallengeManagedAt200, StatusCode: 200, RayID: rayID}
		}
		if strings.Contains(body, "cf-turnstile") || strings.Contains(body, "turnstile-wrapper") {
			return &CloudflareError{Type: ChallengeTurnstile, StatusCode: 200, RayID: rayID}
		}
		if strings.Contains(body, "_cf_chl_opt") || strings.Contains(body, "challenge-platform") {
			return &CloudflareError{Type: ChallengeManagedAt200, StatusCode: 200, RayID: rayID}
		}
	}

	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/krolik/src/go-stealth && go test -run TestDetectCloudflare -v`
Expected: PASS (all existing + 5 new)

**Step 5: Commit**

```bash
cd /home/krolik/src/go-stealth
git add cloudflare.go cloudflare_200_test.go
git commit -m "feat: detect CF challenges at HTTP 200 (cf-mitigated, _cf_chl_opt)"
```

---

### Task 4: SmartFetchMiddleware

Middleware that delegates CF-challenged requests to ox-browser `/fetch-smart`. Returns the solved body directly (no second request needed from go-stealth).

**Dependencies:** Task 1

**Files:**
- Create: `middleware_oxbrowser.go`
- Create: `middleware_oxbrowser_test.go`

**Step 1: Write the failing test**

```go
// middleware_oxbrowser_test.go
package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSmartFetchMiddleware_NoCF(t *testing.T) {
	// No CF — middleware should pass through.
	base := func(req *Request) (*Response, error) {
		return &Response{Body: []byte("ok"), StatusCode: 200, Headers: map[string]string{}}, nil
	}
	mw := SmartFetchMiddleware(NewOxBrowserClient("http://unused"))
	handler := mw(base)

	resp, err := handler(&Request{Method: "GET", URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("expected passthrough, got %q", resp.Body)
	}
}

func TestSmartFetchMiddleware_CFDetected(t *testing.T) {
	// Simulate CF 503 from base handler, ox-browser solves it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      200,
			"body":        "<html>solved</html>",
			"method":      "solved",
			"cf_detected": true,
			"elapsed_ms":  1500,
		})
	}))
	defer srv.Close()

	base := func(req *Request) (*Response, error) {
		return &Response{
			Body:       []byte(`<script src="/cdn-cgi/challenge-platform/x.js"></script>`),
			StatusCode: 503,
			Headers:    map[string]string{"server": "cloudflare"},
		}, nil
	}

	mw := SmartFetchMiddleware(NewOxBrowserClient(srv.URL))
	handler := mw(base)

	resp, err := handler(&Request{Method: "GET", URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "<html>solved</html>" {
		t.Errorf("expected solved body, got %q", resp.Body)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSmartFetchMiddleware_CFBlock(t *testing.T) {
	// Block challenge — should return error, not call ox-browser.
	base := func(req *Request) (*Response, error) {
		return &Response{
			Body:       []byte("you have been blocked"),
			StatusCode: 403,
			Headers:    map[string]string{"server": "cloudflare"},
		}, nil
	}

	mw := SmartFetchMiddleware(NewOxBrowserClient("http://unused"))
	handler := mw(base)

	resp, err := handler(&Request{Method: "GET", URL: "https://example.com"})
	if err == nil {
		t.Error("expected error for block challenge")
	}
	if resp == nil || resp.StatusCode != 403 {
		t.Errorf("expected 403 response, got %v", resp)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/krolik/src/go-stealth && go test -run TestSmartFetchMiddleware -v`
Expected: FAIL — `SmartFetchMiddleware` undefined

**Step 3: Write minimal implementation**

```go
// middleware_oxbrowser.go
package stealth

import (
	"context"
	"log/slog"
)

// SmartFetchMiddleware returns a middleware that detects Cloudflare challenges
// and delegates to ox-browser /fetch-smart for automatic solving.
// On CF detection, the original response is replaced with ox-browser's solved response.
// Non-CF responses pass through unchanged.
func SmartFetchMiddleware(oxClient *OxBrowserClient) Middleware {
	return func(next Handler) Handler {
		return func(req *Request) (*Response, error) {
			resp, err := next(req)
			if err != nil {
				return resp, err
			}

			cfErr := DetectCloudflare(resp)
			if cfErr == nil {
				return resp, nil
			}

			if cfErr.Type == ChallengeBlock {
				return resp, cfErr
			}

			slog.Debug("smartfetch: CF detected, delegating to ox-browser",
				slog.String("url", req.URL),
				slog.String("type", string(cfErr.Type)),
			)

			oxResp, oxErr := oxClient.FetchSmart(context.Background(), req.URL)
			if oxErr != nil {
				slog.Warn("smartfetch: ox-browser failed, returning original",
					slog.Any("error", oxErr))
				return resp, cfErr
			}

			if oxResp.Error != "" {
				return resp, cfErr
			}

			return &Response{
				Body:       []byte(oxResp.Body),
				StatusCode: oxResp.Status,
				Headers:    resp.Headers,
			}, nil
		}
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/krolik/src/go-stealth && go test -run TestSmartFetchMiddleware -v`
Expected: PASS (3 tests)

**Step 5: Commit**

```bash
cd /home/krolik/src/go-stealth
git add middleware_oxbrowser.go middleware_oxbrowser_test.go
git commit -m "feat: add SmartFetchMiddleware — delegate CF to ox-browser"
```

---

### Task 5: SiteIntel — Per-Domain Tech/WAF Cache

Calls ox-browser `/analyze` once per domain, caches results, exposes WAF detection and tech-aware profile selection.

**Dependencies:** Task 1

**Files:**
- Create: `site_intel.go`
- Create: `site_intel_test.go`

**Step 1: Write the failing test**

```go
// site_intel_test.go
package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSiteIntel_DetectsWAF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"url":    "https://example.com",
			"status": 200,
			"technologies": []map[string]interface{}{
				{"name": "Cloudflare", "categories": []string{"CDN"}, "confidence": 100},
				{"name": "React", "categories": []string{"JavaScript frameworks"}, "confidence": 100},
			},
		})
	}))
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	info, err := intel.Get("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if info.WAF != "Cloudflare" {
		t.Errorf("expected WAF=Cloudflare, got %q", info.WAF)
	}
	if !info.HasTech("React") {
		t.Error("expected React in tech stack")
	}
}

func TestSiteIntel_CachesResults(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"url": "https://example.com", "status": 200,
			"technologies": []map[string]interface{}{},
		})
	}))
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	intel.Get("https://example.com")
	intel.Get("https://example.com")
	if calls.Load() != 1 {
		t.Errorf("expected 1 API call (cached), got %d", calls.Load())
	}
}

func TestSiteIntel_SuggestProfile_Cloudflare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"url": "https://example.com", "status": 200,
			"technologies": []map[string]interface{}{
				{"name": "Cloudflare", "categories": []string{"CDN"}, "confidence": 100},
			},
		})
	}))
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	profile := intel.SuggestProfile("https://example.com")
	// Cloudflare sites should get Chrome profile (most common, least suspicious).
	if profile.Browser != "chrome" {
		t.Errorf("expected chrome for Cloudflare site, got %s", profile.Browser)
	}
}

func TestSiteIntel_DetectsAkamai(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"url": "https://example.com", "status": 200,
			"technologies": []map[string]interface{}{
				{"name": "Akamai", "categories": []string{"CDN"}, "confidence": 100},
			},
		})
	}))
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	info, _ := intel.Get("https://example.com")
	if info.WAF != "Akamai" {
		t.Errorf("expected WAF=Akamai, got %q", info.WAF)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/krolik/src/go-stealth && go test -run TestSiteIntel -v`
Expected: FAIL — `NewSiteIntel` undefined

**Step 3: Write minimal implementation**

```go
// site_intel.go
package stealth

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Known WAF/CDN technology names from rswappalyzer.
var knownWAFs = []string{
	"Cloudflare", "Akamai", "Sucuri", "Imperva", "AWS WAF",
	"Fastly", "StackPath", "Barracuda", "F5 BIG-IP",
}

// SiteInfo contains cached intelligence about a domain.
type SiteInfo struct {
	WAF          string        // Detected WAF/CDN name, or ""
	Technologies []AnalyzeTech // Full tech list
	FetchedAt    time.Time
}

// HasTech returns true if the site has the named technology.
func (si *SiteInfo) HasTech(name string) bool {
	for _, t := range si.Technologies {
		if strings.EqualFold(t.Name, name) {
			return true
		}
	}
	return false
}

// SiteIntel provides per-domain tech/WAF intelligence with caching.
type SiteIntel struct {
	client *OxBrowserClient
	ttl    time.Duration

	mu    sync.RWMutex
	cache map[string]*SiteInfo
}

// NewSiteIntel creates a SiteIntel cache backed by ox-browser /analyze.
func NewSiteIntel(client *OxBrowserClient) *SiteIntel {
	return &SiteIntel{
		client: client,
		ttl:    1 * time.Hour,
		cache:  make(map[string]*SiteInfo),
	}
}

// Get returns cached SiteInfo for the domain, fetching via /analyze if needed.
func (si *SiteIntel) Get(rawURL string) (*SiteInfo, error) {
	domain := extractDomain(rawURL)

	si.mu.RLock()
	if info, ok := si.cache[domain]; ok && time.Since(info.FetchedAt) < si.ttl {
		si.mu.RUnlock()
		return info, nil
	}
	si.mu.RUnlock()

	resp, err := si.client.Analyze(context.Background(), rawURL)
	if err != nil {
		slog.Debug("site_intel: analyze failed", slog.String("url", rawURL), slog.Any("error", err))
		return &SiteInfo{}, err
	}

	info := &SiteInfo{
		Technologies: resp.Technologies,
		FetchedAt:    time.Now(),
	}
	for _, t := range resp.Technologies {
		for _, waf := range knownWAFs {
			if strings.EqualFold(t.Name, waf) {
				info.WAF = waf
				break
			}
		}
		if info.WAF != "" {
			break
		}
	}

	si.mu.Lock()
	si.cache[domain] = info
	si.mu.Unlock()

	return info, nil
}

// SuggestProfile returns a BrowserProfile optimized for the target site's WAF.
func (si *SiteIntel) SuggestProfile(rawURL string) BrowserProfile {
	info, err := si.Get(rawURL)
	if err != nil {
		return RandomProfile()
	}

	switch info.WAF {
	case "Cloudflare":
		// Chrome is most common on CF sites — least suspicious.
		return RandomProfile(WithBrowser("chrome"), WithMobile(false))
	case "Akamai":
		// Akamai checks TLS fingerprint aggressively — randomize browser.
		return RandomProfile(WithMobile(false))
	default:
		return RandomProfile()
	}
}
```

**Step 4: Run test to verify it passes**

Run: `cd /home/krolik/src/go-stealth && go test -run TestSiteIntel -v`
Expected: PASS (4 tests)

**Step 5: Commit**

```bash
cd /home/krolik/src/go-stealth
git add site_intel.go site_intel_test.go
git commit -m "feat: add SiteIntel — per-domain tech/WAF cache via ox-browser"
```

---

### Task 6: WithOxBrowser Client Option

Wire everything together: new `WithOxBrowser(url)` option that enables OxBrowserSolver + SmartFetchMiddleware.

**Dependencies:** Tasks 2, 3, 4

**Files:**
- Modify: `client_options.go` (add `WithOxBrowser` option + `oxBrowserURL` field)
- Modify: `client.go` (wire ox-browser middlewares in `NewClient`)
- Create: `client_oxbrowser_test.go`

**Step 1: Write the failing test**

```go
// client_oxbrowser_test.go
package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithOxBrowser_Integration(t *testing.T) {
	// Mock ox-browser /solve endpoint.
	oxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/solve" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "ok",
				"cookies": map[string]string{"cf_clearance": "integ-test"},
			})
			return
		}
		if r.URL.Path == "/fetch-smart" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": 200, "body": "<html>solved</html>",
				"method": "solved", "cf_detected": true, "elapsed_ms": 100,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer oxSrv.Close()

	client, err := NewClient(
		WithStdHTTP(),
		WithOxBrowser(oxSrv.URL),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = client // Client created successfully with ox-browser integration.
}
```

**Step 2: Run test to verify it fails**

Run: `cd /home/krolik/src/go-stealth && go test -run TestWithOxBrowser -v`
Expected: FAIL — `WithOxBrowser` undefined

**Step 3: Modify `client_options.go`**

Add to `clientConfig`:
```go
oxBrowserURL string
```

Add option function:
```go
// WithOxBrowser enables ox-browser integration for CF solving and smart fetch.
// Adds OxBrowserSolver (CookieProvider) and SmartFetchMiddleware.
// url is the ox-browser base URL (e.g. "http://127.0.0.1:8901").
func WithOxBrowser(url string) ClientOption {
	return func(c *clientConfig) {
		c.oxBrowserURL = url
	}
}
```

**Step 4: Modify `client.go` — wire in NewClient**

After existing `cookieProvider` block (~line 78), add:
```go
if cfg.oxBrowserURL != "" {
	oxClient := NewOxBrowserClient(cfg.oxBrowserURL)
	// If no explicit CookieProvider, use OxBrowserSolver.
	if cfg.cookieProvider == nil {
		bc.Use(CloudflareCookieMiddleware(NewOxBrowserSolver(OxBrowserSolverConfig{
			BaseURL: cfg.oxBrowserURL,
		})))
		bc.Use(CloudflareDetectMiddleware)
	}
	// Add SmartFetch as fallback after CF detect middleware.
	bc.Use(SmartFetchMiddleware(oxClient))
}
```

**Step 5: Run test to verify it passes**

Run: `cd /home/krolik/src/go-stealth && go test -run TestWithOxBrowser -v`
Expected: PASS

**Step 6: Run all tests**

Run: `cd /home/krolik/src/go-stealth && go test ./... -v`
Expected: ALL PASS

**Step 7: Commit**

```bash
cd /home/krolik/src/go-stealth
git add client_options.go client.go client_oxbrowser_test.go
git commit -m "feat: add WithOxBrowser option — unified CF solving + smart fetch"
```

---

### Task 7: Lint & Final Verification

**Dependencies:** All previous tasks

**Step 1: Run linter**

Run: `cd /home/krolik/src/go-stealth && golangci-lint run`
Expected: No errors

**Step 2: Run full test suite**

Run: `cd /home/krolik/src/go-stealth && go test ./... -count=1 -v`
Expected: ALL PASS

**Step 3: Verify file sizes**

Run: `wc -l oxbrowser.go solver_oxbrowser.go middleware_oxbrowser.go site_intel.go cloudflare.go client.go client_options.go`
Expected: Each file ≤ 200 lines

**Step 4: Tag release**

```bash
cd /home/krolik/src/go-stealth
git tag v1.6.0
```

---

## Task Dependency Graph

```
Task 1 (OxBrowserClient)
  ├── Task 2 (OxBrowserSolver) ──┐
  ├── Task 4 (SmartFetchMiddleware)──┤
  └── Task 5 (SiteIntel)        │
                                 │
Task 3 (Enhanced CF Detection)───┤
                                 │
                         Task 6 (WithOxBrowser) ── Task 7 (Lint & Verify)
```

**Parallel groups:**
- **Group A (independent):** Tasks 2, 3, 4, 5 can all run in parallel after Task 1
- **Group B (sequential):** Task 6 after Group A, Task 7 after Task 6
