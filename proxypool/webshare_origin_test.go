package proxypool

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWebshareFetch_CrossOriginNextNotFollowed(t *testing.T) {
	var otherHits atomic.Int32
	var otherAuth atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		otherAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"next":null}`))
	}))
	defer other.Close()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"proxy_address":"1.1.1.1","port":8080,"username":"u","password":"p"}],"next":"` + other.URL + `/page2"}`))
	}))
	defer first.Close()

	_, err := NewWebshareWithConfig("secret-key", WebshareConfig{Countries: []string{"US"}, BaseURL: first.URL})
	if err == nil {
		t.Fatal("expected an error when next points to another origin")
	}
	if n := otherHits.Load(); n != 0 {
		t.Fatalf("cross-origin server received %d request(s) (Authorization=%v); credentials must not leave the first origin", n, otherAuth.Load())
	}
}

func TestWebshareFetch_RedirectNotFollowed(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"proxy_address":"1.1.1.1","port":8080,"username":"u","password":"p"}],"next":null}`))
	}))
	defer other.Close()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/x", http.StatusFound)
	}))
	defer first.Close()

	_, err := NewWebshareWithConfig("secret-key", WebshareConfig{Countries: []string{"US"}, BaseURL: first.URL})
	if err == nil {
		t.Fatal("expected an error on redirect")
	}
	if n := otherHits.Load(); n != 0 {
		t.Fatalf("redirect target received %d request(s); redirects must not be followed with credentials", n)
	}
}

func TestWebshareFetch_SameOriginNextFollowedWithKey(t *testing.T) {
	var srvURL string
	var page2Auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/page2" {
			page2Auth.Store(r.Header.Get("Authorization"))
			_, _ = w.Write([]byte(`{"results":[{"proxy_address":"2.2.2.2","port":8080,"username":"u","password":"p"}],"next":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"proxy_address":"1.1.1.1","port":8080,"username":"u","password":"p"}],"next":"` + srvURL + `/page2"}`))
	}))
	defer srv.Close()
	srvURL = srv.URL

	pool, err := NewWebshareWithConfig("secret-key", WebshareConfig{Countries: []string{"US"}, BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool.Len() != 2 {
		t.Fatalf("expected 2 proxies across both pages, got %d", pool.Len())
	}
	if got := page2Auth.Load(); got != "Token secret-key" {
		t.Fatalf("page 2 Authorization = %v, want the key", got)
	}
}

func TestWebshareRotating_RedirectNotFollowed(t *testing.T) {
	var otherHits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"proxy_address":"1.1.1.1","port":8080,"username":"u","password":"p"}],"next":null}`))
	}))
	defer other.Close()

	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/x", http.StatusFound)
	}))
	defer first.Close()

	_, err := NewWebshareWithConfig("secret-key", WebshareConfig{Mode: ModeRotating, Countries: []string{"US"}, BaseURL: first.URL})
	if err == nil {
		t.Fatal("expected an error on redirect in rotating mode")
	}
	if n := otherHits.Load(); n != 0 {
		t.Fatalf("redirect target (same host, other port) received %d request(s) in rotating mode", n)
	}
}

func TestCheckSameOrigin(t *testing.T) {
	const first = "https://api.example.com/v2/proxy/list/?page_size=100"
	cases := []struct {
		name string
		next string
		ok   bool
	}{
		{"same origin", "https://api.example.com/v2/proxy/list/?page=2", true},
		{"scheme differs", "http://api.example.com/v2/proxy/list/?page=2", false},
		{"scheme differs same port", "http://api.example.com:443/v2/proxy/list/?page=2", false},
		{"port differs", "https://api.example.com:8443/v2/proxy/list/?page=2", false},
		{"host differs", "https://evil.example.net/v2/proxy/list/?page=2", false},
		{"userinfo host", "https://api.example.com@evil.example.net/", false},
		{"uppercase host", "https://API.Example.COM/v2/proxy/list/?page=2", true},
		{"explicit default port", "https://api.example.com:443/v2/proxy/list/?page=2", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkSameOrigin(first, tc.next)
			if tc.ok && err != nil {
				t.Fatalf("checkSameOrigin(%q) rejected a same-origin link: %v", tc.next, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("checkSameOrigin(%q) accepted a cross-origin link", tc.next)
			}
		})
	}
	if err := checkSameOrigin("http://h.example.com/a", "http://h.example.com:80/b"); err != nil {
		t.Fatalf("http explicit :80 rejected: %v", err)
	}
	if err := checkSameOrigin("http://h.example.com:80/a", "http://h.example.com/b"); err != nil {
		t.Fatalf("http implicit port rejected against explicit :80 first: %v", err)
	}
}
