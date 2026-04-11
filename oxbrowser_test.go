package stealth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestNewOxBrowserClientWithProxy_RoutesViaProxy(t *testing.T) {
	proxied := false

	// Mock proxy server: records that a request passed through it, then forwards
	// the request to the real target (also a mock) by acting as a tunnel.
	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "proxied-token"},
		})
	}))
	defer targetSrv.Close()

	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = true
		// Forward the request to the real target server.
		proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetSrv.URL+r.URL.Path, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		proxyReq.Header = r.Header
		resp, err := http.DefaultClient.Do(proxyReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close() //nolint:errcheck
		w.WriteHeader(resp.StatusCode)
		_ = http.NewResponseController(w).Flush()
		buf := make([]byte, 32*1024) //nolint:mnd
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
			}
			if readErr != nil {
				break
			}
		}
	}))
	defer proxySrv.Close()

	proxyURL, _ := url.Parse(proxySrv.URL)
	proxyFn := http.ProxyURL(proxyURL)

	// Client points at targetSrv but should route through proxySrv.
	client := NewOxBrowserClientWithProxy(targetSrv.URL, proxyFn)
	cookies, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err != nil {
		t.Fatal(err)
	}
	if !proxied {
		t.Error("expected request to route through proxy, but proxy was not hit")
	}
	if cookies["cf_clearance"] != "proxied-token" {
		t.Errorf("unexpected cookies: %v", cookies)
	}
}
