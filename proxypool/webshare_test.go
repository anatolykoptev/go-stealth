package proxypool

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// --- Legacy tests (unchanged behaviour, use newWebshareFromURL) ---

func TestNewWebshare_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"proxy_address": "1.2.3.4", "port": 8080, "username": "user1", "password": "pass1"},
				{"proxy_address": "5.6.7.8", "port": 9090, "username": "user2", "password": "pass2"}
			],
			"next": null
		}`))
	}))
	defer server.Close()

	pool, err := newWebshareFromURL(server.URL, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.Len() != 2 {
		t.Fatalf("expected 2 proxies, got %d", pool.Len())
	}
}

func TestNewWebshare_EmptyKey(t *testing.T) {
	_, err := NewWebshare("")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestNewWebshare_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Invalid token."}`))
	}))
	defer server.Close()

	_, err := newWebshareFromURL(server.URL, "bad-key")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestNewWebshare_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results": [], "next": null}`))
	}))
	defer server.Close()

	_, err := newWebshareFromURL(server.URL, "test-key")
	if err == nil {
		t.Fatal("expected error for empty results")
	}
}

func TestNewWebshare_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	_, err := newWebshareFromURL(server.URL, "test-key")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestWebshare_RoundRobin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"proxy_address": "1.1.1.1", "port": 1111, "username": "u", "password": "p"},
				{"proxy_address": "2.2.2.2", "port": 2222, "username": "u", "password": "p"},
				{"proxy_address": "3.3.3.3", "port": 3333, "username": "u", "password": "p"}
			],
			"next": null
		}`))
	}))
	defer server.Close()

	pool, err := newWebshareFromURL(server.URL, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First call: counter wraps to index 1 (counter starts at 0, Add(1)=1, 1%3=1)
	first := pool.Next()
	second := pool.Next()
	third := pool.Next()
	fourth := pool.Next()

	if first == second && second == third {
		t.Fatal("round-robin should return different proxies")
	}
	if first != fourth {
		t.Fatalf("expected rotation back to first proxy, got %s vs %s", first, fourth)
	}
}

func TestWebshare_TransportProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"proxy_address": "10.0.0.1", "port": 8080, "username": "u", "password": "p"}
			],
			"next": null
		}`))
	}))
	defer server.Close()

	pool, err := newWebshareFromURL(server.URL, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fn := pool.TransportProxy()
	if fn == nil {
		t.Fatal("TransportProxy should not return nil")
	}

	proxyURL, err := fn(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proxyURL.Hostname() != "10.0.0.1" {
		t.Fatalf("expected host 10.0.0.1, got %s", proxyURL.Hostname())
	}
}

// --- New country-targeting tests ---

func TestNewWebshare_DefaultCountryUS(t *testing.T) {
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"proxy_address": "1.2.3.4", "port": 8080, "username": "user1", "password": "pass"}
			],
			"next": null
		}`))
	}))
	defer server.Close()

	pool, err := NewWebshareWithConfig("test-key", WebshareConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedURL, "country_code__in=US") {
		t.Errorf("expected country_code__in=US in request URL, got: %s", capturedURL)
	}

	proxy := pool.Next()
	if !strings.Contains(proxy, "user1-US") {
		t.Errorf("expected username suffix -US, got: %s", proxy)
	}
}

func TestNewWebshareWithConfig_MultiCountry(t *testing.T) {
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"proxy_address": "1.2.3.4", "port": 8080, "username": "user1", "password": "pass"}
			],
			"next": null
		}`))
	}))
	defer server.Close()

	pool, err := NewWebshareWithConfig("test-key", WebshareConfig{
		Countries: []string{"US", "GB"},
		BaseURL:   server.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedURL, "country_code__in=US,GB") {
		t.Errorf("expected country_code__in=US,GB in URL, got: %s", capturedURL)
	}

	// One proxy × two countries = 2 pool entries.
	if pool.Len() != 2 {
		t.Fatalf("expected 2 entries (1 proxy × 2 countries), got %d", pool.Len())
	}

	// Round-robin should give us both countries.
	seen := map[string]bool{}
	for range 4 {
		p := pool.Next()
		if strings.Contains(p, "-US") {
			seen["US"] = true
		}
		if strings.Contains(p, "-GB") {
			seen["GB"] = true
		}
	}
	if !seen["US"] || !seen["GB"] {
		t.Errorf("expected both -US and -GB suffixes in round-robin, got seen=%v", seen)
	}
}

func TestNewWebshareWithConfig_InvalidCountry(t *testing.T) {
	_, err := NewWebshareWithConfig("test-key", WebshareConfig{
		Countries: []string{"usa"}, // lowercase, 3-char — invalid
	})
	if err == nil {
		t.Fatal("expected error for invalid country code")
	}
	if !strings.Contains(err.Error(), "invalid country code") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewWebshareWithConfig_Pagination(t *testing.T) {
	page2URL := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "page=2") {
			// Page 2: no next
			_, _ = w.Write([]byte(`{
				"results": [
					{"proxy_address": "5.5.5.5", "port": 9090, "username": "user3", "password": "pass"}
				],
				"next": null
			}`))
		} else {
			// Page 1: has next
			_, _ = w.Write([]byte(`{
				"results": [
					{"proxy_address": "1.1.1.1", "port": 8080, "username": "user1", "password": "pass"},
					{"proxy_address": "2.2.2.2", "port": 8080, "username": "user2", "password": "pass"}
				],
				"next": "` + page2URL + `"
			}`))
		}
	}))
	defer server.Close()
	page2URL = server.URL + "?mode=backbone&page_size=100&country_code__in=US&page=2"

	// Rebuild handler with known page2URL (server already started, so patch after).
	// We use a simple mux instead.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{
				"results": [
					{"proxy_address": "5.5.5.5", "port": 9090, "username": "user3", "password": "pass"}
				],
				"next": null
			}`))
			return
		}
		// We need a server URL for next — use the server from the closure below.
		_, _ = w.Write([]byte(`{
			"results": [
				{"proxy_address": "1.1.1.1", "port": 8080, "username": "user1", "password": "pass"},
				{"proxy_address": "2.2.2.2", "port": 8080, "username": "user2", "password": "pass"}
			],
			"next": null
		}`))
	})

	// Use a fresh server with controlled next URL.
	var nextURL string
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"results":[` +
				`{"proxy_address":"1.1.1.1","port":8080,"username":"user1","password":"pass"},` +
				`{"proxy_address":"2.2.2.2","port":8080,"username":"user2","password":"pass"}` +
				`],"next":"` + nextURL + `"}`))
		} else {
			_, _ = w.Write([]byte(`{"results":[` +
				`{"proxy_address":"5.5.5.5","port":9090,"username":"user3","password":"pass"}` +
				`],"next":null}`))
		}
	}))
	defer srv.Close()
	nextURL = srv.URL + "/page2"

	pool, err := NewWebshareWithConfig("test-key", WebshareConfig{
		Countries: []string{"US"},
		BaseURL:   srv.URL,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 3 base proxies × 1 country = 3 entries.
	if pool.Len() != 3 {
		t.Fatalf("expected 3 proxies (2 page1 + 1 page2), got %d", pool.Len())
	}
}

func TestNewWebshareRotating_NoAPIKey(t *testing.T) {
	pool, err := NewWebshareRotating("testuser", "secret", "US", "GB")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", pool.Len())
	}

	seen := map[string]bool{}
	for range 4 {
		p := pool.Next()
		parsed, err := url.Parse(p)
		if err != nil {
			t.Fatalf("failed to parse proxy URL: %v", err)
		}
		if parsed.Hostname() != webshareDefaultHost {
			t.Errorf("expected host %s, got %s", webshareDefaultHost, parsed.Hostname())
		}
		if parsed.Port() != "80" {
			t.Errorf("expected port 80, got %s", parsed.Port())
		}
		if strings.Contains(p, "testuser-US-rotate") {
			seen["US"] = true
		}
		if strings.Contains(p, "testuser-GB-rotate") {
			seen["GB"] = true
		}
	}
	if !seen["US"] || !seen["GB"] {
		t.Errorf("expected both US and GB rotating URLs, seen=%v", seen)
	}
}

func TestNewWebshareRotating_DefaultCountry(t *testing.T) {
	pool, err := NewWebshareRotating("testuser", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.Len() != 1 {
		t.Fatalf("expected 1 entry (default US), got %d", pool.Len())
	}

	proxy := pool.Next()
	if !strings.Contains(proxy, "testuser-US-rotate") {
		t.Errorf("expected -US-rotate in proxy URL, got: %s", proxy)
	}
}

func TestNewWebshareWithConfig_BackCompatOldTest(t *testing.T) {
	// Verifies that the legacy newWebshareFromURL helper still works without country defaults.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results": [
				{"proxy_address": "1.2.3.4", "port": 8080, "username": "user1", "password": "pass1"},
				{"proxy_address": "5.6.7.8", "port": 9090, "username": "user2", "password": "pass2"}
			],
			"next": null
		}`))
	}))
	defer server.Close()

	pool, err := newWebshareFromURL(server.URL, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.Len() != 2 {
		t.Fatalf("expected 2 proxies, got %d", pool.Len())
	}

	// No country suffix — raw username.
	proxy := pool.Next()
	if strings.Contains(proxy, "-US") {
		t.Errorf("newWebshareFromURL should NOT inject country suffix, got: %s", proxy)
	}
}

func TestValidateAndDedup_Deduplication(t *testing.T) {
	result, err := validateAndDedup([]string{"US", "GB", "US", "DE", "GB"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 unique countries, got %d: %v", len(result), result)
	}
	if result[0] != "US" || result[1] != "GB" || result[2] != "DE" {
		t.Errorf("unexpected dedup order: %v", result)
	}
}
