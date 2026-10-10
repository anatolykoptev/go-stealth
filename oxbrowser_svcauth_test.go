package stealth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestOxBrowserClient_SendsInternalSecret — with INTERNAL_SERVICE_SECRET set,
// requests from NewOxBrowserClient to ox-browser carry X-Internal-Secret.
func TestOxBrowserClient_SendsInternalSecret(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_SECRET", "s3cret")

	var gotSecret string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSecret = r.Header.Get("X-Internal-Secret")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "tok"},
		})
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	if _, err := client.Solve(context.Background(), "https://example.com", "js_challenge"); err != nil {
		t.Fatal(err)
	}
	if gotSecret != "s3cret" {
		t.Errorf("expected X-Internal-Secret=s3cret at ox-browser, got %q", gotSecret)
	}
}

// TestOxBrowserSolver_SendsInternalSecret — the solver's embedded client also
// authenticates to ox-browser when the env var is set.
func TestOxBrowserSolver_SendsInternalSecret(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_SECRET", "s3cret")

	var gotSecret string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSecret = r.Header.Get("X-Internal-Secret")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "tok"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL})
	if _, err := solver.Solve("example.com", nil); err != nil {
		t.Fatal(err)
	}
	if gotSecret != "s3cret" {
		t.Errorf("expected X-Internal-Secret=s3cret at ox-browser, got %q", gotSecret)
	}
}

// TestWithOxBrowser_SendsInternalSecret — the consumer path: NewClient with
// WithOxBrowser must send the secret on its SmartFetch call to ox-browser.
func TestWithOxBrowser_SendsInternalSecret(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_SECRET", "s3cret")

	var gotSecret string
	oxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSecret = r.Header.Get("X-Internal-Secret")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 200, "body": "<html>solved</html>",
			"method": "solved", "cf_detected": true, "elapsed_ms": 100,
		})
	}))
	defer oxSrv.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("server", "cloudflare")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<script src="/cdn-cgi/challenge-platform/x.js"></script>`))
	}))
	defer target.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithStdHTTP(), WithOxBrowser(oxSrv.URL))
	if err != nil {
		t.Fatal(err)
	}
	body, _, _, err := client.Do(http.MethodGet, target.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "<html>solved</html>" {
		t.Fatalf("expected solved body from ox-browser, got %q", body)
	}
	if gotSecret != "s3cret" {
		t.Errorf("expected X-Internal-Secret=s3cret at ox-browser, got %q", gotSecret)
	}
}

// TestOxBrowserClient_RedirectToOtherOriginStripsSecret — a redirect from the
// ox-browser origin to a different origin must not carry the secret.
func TestOxBrowserClient_RedirectToOtherOriginStripsSecret(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_SECRET", "s3cret")

	var secondSecret string
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondSecret = r.Header.Get("X-Internal-Secret")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "redirected"},
		})
	}))
	defer second.Close()

	var firstSecret string
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstSecret = r.Header.Get("X-Internal-Secret")
		http.Redirect(w, r, second.URL+"/solve", http.StatusFound)
	}))
	defer first.Close()

	client := NewOxBrowserClient(first.URL)
	cookies, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err != nil {
		t.Fatal(err)
	}
	if cookies["cf_clearance"] != "redirected" {
		t.Fatalf("redirect was not followed to second origin, cookies=%v", cookies)
	}
	if firstSecret != "s3cret" {
		t.Errorf("expected X-Internal-Secret=s3cret at routed origin, got %q", firstSecret)
	}
	if secondSecret != "" {
		t.Errorf("secret leaked to redirected origin: got %q", secondSecret)
	}
}

// TestOxBrowserClient_NoSecretWhenEnvUnset — with the env var unset the client
// sends no X-Internal-Secret and requests still go through.
func TestOxBrowserClient_NoSecretWhenEnvUnset(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_SECRET", "")

	called := false
	var gotSecret string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotSecret = r.Header.Get("X-Internal-Secret")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "tok"},
		})
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	if _, err := client.Solve(context.Background(), "https://example.com", "js_challenge"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("request never reached ox-browser stand-in")
	}
	if gotSecret != "" {
		t.Errorf("expected no X-Internal-Secret with env unset, got %q", gotSecret)
	}
}

// TestNewOxBrowserClientWithProxy_NoSecretThroughProxy — the proxied client
// must NOT attach the secret: the request to ox-browser itself transits the
// external proxy, which would see the header.
func TestNewOxBrowserClientWithProxy_NoSecretThroughProxy(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_SECRET", "s3cret")

	proxied := false
	var proxySawSecret string
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = true
		proxySawSecret = r.Header.Get("X-Internal-Secret")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "via-proxy"},
		})
	}))
	defer proxySrv.Close()

	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached ox-browser target directly, expected proxy path")
	}))
	defer targetSrv.Close()

	proxyURL, _ := url.Parse(proxySrv.URL)
	client := NewOxBrowserClientWithProxy(targetSrv.URL, http.ProxyURL(proxyURL))
	cookies, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err != nil {
		t.Fatal(err)
	}
	if !proxied {
		t.Fatal("request did not transit the proxy")
	}
	if cookies["cf_clearance"] != "via-proxy" {
		t.Fatalf("unexpected cookies: %v", cookies)
	}
	if proxySawSecret != "" {
		t.Errorf("secret leaked to external proxy: got %q", proxySawSecret)
	}
}
