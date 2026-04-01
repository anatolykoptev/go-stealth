package stealth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestBrowserClient_DoCtx_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = client.DoCtx(ctx, "GET", "http://127.0.0.1:1", nil, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestBrowserClient_DoCtx_Timeout(t *testing.T) {
	// Server that hangs until the test ends.
	unblock := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-unblock
		w.WriteHeader(200)
	}))
	defer server.Close()
	defer close(unblock)

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, _, err = client.DoCtx(ctx, "GET", server.URL, nil, nil)
	if err == nil {
		t.Fatal("expected error for timed-out context, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestBrowserClient_DoWithHeaderOrderCtx_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = client.DoWithHeaderOrderCtx(ctx, "GET", "http://127.0.0.1:1", nil, nil, DefaultHeaderOrder)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}
