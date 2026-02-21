package stealth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserClient_Do(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "hello")
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	body, headers, status, err := client.Do("GET", server.URL, map[string]string{"user-agent": "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", body)
	}
	if headers["x-test"] != "hello" {
		t.Fatalf("expected x-test header, got: %v", headers)
	}
}

func TestBrowserClient_DoPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Write(b)
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	body, _, status, err := client.Do("POST", server.URL, nil, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if string(body) != "hello" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestBrowserClient_SetCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "test_cookie", Value: "abc123"})
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	_, headers, _, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(headers["set-cookie"], "test_cookie=abc123") {
		t.Fatalf("expected cookie header, got: %v", headers)
	}
}
