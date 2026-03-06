package stealth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSmartFetch_HandlerError: base handler returns error (not CF) — middleware must
// pass the error through unchanged without calling ox-browser.
func TestSmartFetch_HandlerError(t *testing.T) {
	t.Parallel()

	baseErr := fmt.Errorf("connection reset by peer")
	base := func(req *Request) (*Response, error) {
		return nil, baseErr
	}

	mw := SmartFetchMiddleware(NewOxBrowserClient("http://unused"))
	handler := mw(base)

	resp, err := handler(&Request{Method: http.MethodGet, URL: "https://example.com"})
	if !errors.Is(err, baseErr) {
		t.Errorf("expected base error to propagate unchanged, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response on base error, got %+v", resp)
	}
}

// TestSmartFetch_OxBrowserReturnsError: ox-browser FetchSmart response has Error field set
// — middleware must return the original CF response + original CF error, not panic or swallow.
func TestSmartFetch_OxBrowserReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 0,
			"body":   "",
			"error":  "chromium crashed unexpectedly",
		})
	}))
	defer srv.Close()

	base := func(req *Request) (*Response, error) {
		return &Response{
			Body:       []byte(`<script src="/cdn-cgi/challenge-platform/x.js"></script>`),
			StatusCode: 503,
			Headers:    map[string]string{"server": "cloudflare", "cf-ray": "abc-LAX"},
		}, nil
	}

	mw := SmartFetchMiddleware(NewOxBrowserClient(srv.URL))
	handler := mw(base)

	resp, err := handler(&Request{Method: http.MethodGet, URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected CloudflareError when ox-browser response has Error field set")
	}
	var cfErr *CloudflareError
	if !errors.As(err, &cfErr) {
		t.Fatalf("expected *CloudflareError, got %T: %v", err, err)
	}
	if cfErr.Type != ChallengeJS {
		t.Errorf("Type = %q, want %q", cfErr.Type, ChallengeJS)
	}
	// Original response must be preserved (not ox-browser's empty body).
	if resp == nil || resp.StatusCode != 503 {
		t.Errorf("expected original 503 response to be returned, got %v", resp)
	}
}

// TestSmartFetch_OxBrowserUnreachable: ox-browser at unreachable address — middleware
// must gracefully fall back: return original response + original CF error.
func TestSmartFetch_OxBrowserUnreachable(t *testing.T) {
	t.Parallel()

	base := func(req *Request) (*Response, error) {
		return &Response{
			Body:       []byte(`<script src="/cdn-cgi/challenge-platform/x.js"></script>`),
			StatusCode: 503,
			Headers:    map[string]string{"server": "cloudflare"},
		}, nil
	}

	// Use a closed httptest server to guarantee immediate "connection refused".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	mw := SmartFetchMiddleware(NewOxBrowserClient(srv.URL))
	handler := mw(base)

	resp, err := handler(&Request{Method: http.MethodGet, URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error when ox-browser is unreachable")
	}
	var cfErr *CloudflareError
	if !errors.As(err, &cfErr) {
		t.Fatalf("expected original *CloudflareError on ox-browser failure, got %T: %v", err, err)
	}
	if resp == nil || resp.StatusCode != 503 {
		t.Errorf("expected original response to be returned on ox-browser failure, got %v", resp)
	}
}

// TestSmartFetch_NilHeaders: original CF response has nil Headers map — middleware
// must not panic. nil Headers → DetectCloudflare returns nil → passthrough.
func TestSmartFetch_NilHeaders(t *testing.T) {
	t.Parallel()

	base := func(req *Request) (*Response, error) {
		return &Response{
			Body:       []byte(`<script src="/cdn-cgi/challenge-platform/x.js"></script>`),
			StatusCode: 503,
			Headers:    nil, // nil map: DetectCloudflare reads resp.Headers["server"] → ""
		}, nil
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic with nil Headers: %v", r)
		}
	}()

	mw := SmartFetchMiddleware(NewOxBrowserClient("http://unused"))
	handler := mw(base)

	// nil Headers → no "cloudflare" server → DetectCloudflare returns nil → passthrough
	resp, err := handler(&Request{Method: http.MethodGet, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil passthrough response")
	}
}

// TestSmartFetch_NilHeaders_SolvedResponseCopy: SmartFetchMiddleware builds the solved
// response as &Response{Headers: resp.Headers}. If the original CF response had a nil
// Headers map, the returned solved response also has nil Headers — verify no panic there.
func TestSmartFetch_NilHeaders_SolvedResponseCopy(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(FetchSmartResponse{
			Status: http.StatusOK,
			Body:   "<html>solved</html>",
			Method: "solved",
		})
	}))
	defer srv.Close()

	// CF-detectable response where Headers exists (for detection) but we
	// want to test the copy path: the returned Response.Headers is resp.Headers.
	// Use an actual CF response; the resulting solved Response will carry the same map.
	base := func(req *Request) (*Response, error) {
		return &Response{
			Body:       []byte(`<script src="/cdn-cgi/challenge-platform/x.js"></script>`),
			StatusCode: 503,
			Headers:    map[string]string{"server": "cloudflare"},
		}, nil
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic when building solved response: %v", r)
		}
	}()

	mw := SmartFetchMiddleware(NewOxBrowserClient(srv.URL))
	handler := mw(base)

	resp, err := handler(&Request{Method: http.MethodGet, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(resp.Body) != "<html>solved</html>" {
		t.Errorf("body = %q, want solved body", resp.Body)
	}
	// Headers in solved response is the original Headers map — verify it's accessible.
	if resp.Headers["server"] != "cloudflare" {
		t.Errorf("solved response Headers should carry original map, got %v", resp.Headers)
	}
}

// TestSmartFetch_PostRequest: method is POST — SmartFetchMiddleware still delegates
// to ox-browser and passes the original request URL.
func TestSmartFetch_PostRequest(t *testing.T) {
	t.Parallel()

	var capturedURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			capturedURL = payload["url"]
		}
		_ = json.NewEncoder(w).Encode(FetchSmartResponse{
			Status: http.StatusOK,
			Body:   "<html>solved</html>",
			Method: "fetch",
		})
	}))
	defer srv.Close()

	const targetURL = "https://example.com/api/data"

	base := func(req *Request) (*Response, error) {
		return &Response{
			Body:       []byte(`<script src="/cdn-cgi/challenge-platform/x.js"></script>`),
			StatusCode: 503,
			Headers:    map[string]string{"server": "cloudflare"},
		}, nil
	}

	mw := SmartFetchMiddleware(NewOxBrowserClient(srv.URL))
	handler := mw(base)

	resp, err := handler(&Request{Method: http.MethodPost, URL: targetURL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedURL != targetURL {
		t.Errorf("ox-browser received URL %q, want %q", capturedURL, targetURL)
	}
	if string(resp.Body) != "<html>solved</html>" {
		t.Errorf("body = %q, want solved body", resp.Body)
	}
}

// TestSmartFetch_ManagedAt200: base returns 200 with cf-mitigated:challenge header
// — DetectCloudflare must identify ChallengeManagedAt200, and middleware must delegate
// to ox-browser (not treat it as a clean pass-through).
func TestSmartFetch_ManagedAt200(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(FetchSmartResponse{
			Status: http.StatusOK,
			Body:   "<html>real page</html>",
			Method: "fetch",
		})
	}))
	defer srv.Close()

	base := func(req *Request) (*Response, error) {
		return &Response{
			Body:       []byte("<html>Just a moment...</html>"),
			StatusCode: http.StatusOK,
			Headers: map[string]string{
				"server":       "cloudflare",
				"cf-mitigated": "challenge",
			},
		}, nil
	}

	mw := SmartFetchMiddleware(NewOxBrowserClient(srv.URL))
	handler := mw(base)

	resp, err := handler(&Request{Method: http.MethodGet, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("expected ox-browser to solve ManagedAt200, got error: %v", err)
	}
	if string(resp.Body) != "<html>real page</html>" {
		t.Errorf("body = %q, want solved page from ox-browser", resp.Body)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
