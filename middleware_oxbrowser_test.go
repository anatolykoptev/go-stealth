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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
