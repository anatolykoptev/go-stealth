package proxypool

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewWebshare_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token test-key" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"results": [
				{"proxy_address": "1.2.3.4", "port": 8080, "username": "user1", "password": "pass1"},
				{"proxy_address": "5.6.7.8", "port": 9090, "username": "user2", "password": "pass2"}
			]
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
		w.WriteHeader(401)
		w.Write([]byte(`{"detail":"Invalid token."}`))
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
		w.Write([]byte(`{"results": []}`))
	}))
	defer server.Close()

	_, err := newWebshareFromURL(server.URL, "test-key")
	if err == nil {
		t.Fatal("expected error for empty results")
	}
}

func TestNewWebshare_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
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
		w.Write([]byte(`{
			"results": [
				{"proxy_address": "1.1.1.1", "port": 1111, "username": "u", "password": "p"},
				{"proxy_address": "2.2.2.2", "port": 2222, "username": "u", "password": "p"},
				{"proxy_address": "3.3.3.3", "port": 3333, "username": "u", "password": "p"}
			]
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
		w.Write([]byte(`{
			"results": [
				{"proxy_address": "10.0.0.1", "port": 8080, "username": "u", "password": "p"}
			]
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
