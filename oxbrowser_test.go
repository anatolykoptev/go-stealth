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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
