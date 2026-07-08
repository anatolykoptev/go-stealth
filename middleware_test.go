package stealth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-stealth/ratelimit"
)

func TestMiddleware_Chain(t *testing.T) {
	var order []string

	mw1 := func(next Handler) Handler {
		return func(req *Request) (*Response, error) {
			order = append(order, "mw1-before")
			resp, err := next(req)
			order = append(order, "mw1-after")
			return resp, err
		}
	}
	mw2 := func(next Handler) Handler {
		return func(req *Request) (*Response, error) {
			order = append(order, "mw2-before")
			resp, err := next(req)
			order = append(order, "mw2-after")
			return resp, err
		}
	}

	base := func(req *Request) (*Response, error) {
		order = append(order, "handler")
		return &Response{StatusCode: 200}, nil
	}

	handler := Chain(mw1, mw2)(base)
	handler(&Request{Method: "GET", URL: "http://test.com"})

	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Fatalf("step %d: expected %q, got %q", i, v, order[i])
		}
	}
}

func TestBrowserClient_Use(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}

	var called bool
	client.Use(func(next Handler) Handler {
		return func(req *Request) (*Response, error) {
			called = true
			return next(req)
		}
	})

	_, _, status, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if !called {
		t.Fatal("middleware was not called")
	}
}

func TestBrowserClient_MiddlewareModifiesHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "injected" {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}

	// Middleware that injects a header
	client.Use(func(next Handler) Handler {
		return func(req *Request) (*Response, error) {
			if req.Headers == nil {
				req.Headers = make(map[string]string)
			}
			req.Headers["X-Custom"] = "injected"
			return next(req)
		}
	})

	_, _, status, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
}

func TestRetryMiddleware(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}

	client.Use(RetryMiddleware(RetryConfig{
		MaxRetries:  3,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     10 * time.Millisecond,
		Multiplier:  1.0,
	}))

	body, _, status, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if string(body) != "success" {
		t.Fatalf("expected 'success', got %q", string(body))
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}

	limiter := ratelimit.NewDomainLimiter(ratelimit.DomainConfig{
		Domain:            "",
		RequestsPerWindow: 2,
		WindowDuration:    1 * time.Minute,
	})

	client.Use(RateLimitMiddlewareWithContext(
		context.Background(),
		limiter,
	))

	// First 2 requests should succeed immediately
	for i := 0; i < 2; i++ {
		_, _, status, err := client.Do("GET", server.URL, nil, nil)
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		if status != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, status)
		}
	}

	// Third request should block — use a short context to timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client2, _ := NewClient(WithoutSSRFGuard())
	client2.Use(RateLimitMiddlewareWithContext(ctx, limiter))

	_, _, _, err = client2.Do("GET", server.URL, nil, nil)
	if err == nil {
		t.Fatal("expected context timeout error")
	}
}

func TestBrowserClient_DoPost_WithMiddleware(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Write(b)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}

	// Add a no-op middleware to ensure POST body passes through
	client.Use(func(next Handler) Handler {
		return func(req *Request) (*Response, error) {
			return next(req)
		}
	})

	body, _, status, err := client.Do("POST", server.URL, nil, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if string(body) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(body))
	}
}

func TestLoggingMiddleware(t *testing.T) {
	base := func(req *Request) (*Response, error) {
		return &Response{StatusCode: 200, Body: []byte("ok")}, nil
	}
	handler := LoggingMiddleware(base)
	resp, err := handler(&Request{Method: "GET", URL: "http://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestClientHintsMiddleware_InjectsHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Sec-Ch-Ua-Platform") == "" {
			w.WriteHeader(400)
			w.Write([]byte("missing sec-ch-ua-platform"))
			return
		}
		if r.Header.Get("Sec-Ch-Ua-Mobile") == "" {
			w.WriteHeader(400)
			w.Write([]byte("missing sec-ch-ua-mobile"))
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	client.Use(ClientHintsMiddleware)

	headers := map[string]string{
		"user-agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	}
	_, _, status, err := client.Do("GET", server.URL, headers, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
}

func TestClientHintsMiddleware_SkipsSafari(t *testing.T) {
	base := func(req *Request) (*Response, error) {
		if _, ok := req.Headers["sec-ch-ua"]; ok {
			return nil, fmt.Errorf("Safari should not have sec-ch-ua")
		}
		return &Response{StatusCode: 200}, nil
	}
	handler := ClientHintsMiddleware(base)
	_, err := handler(&Request{
		Method:  "GET",
		URL:     "http://example.com",
		Headers: map[string]string{"user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientHintsMiddleware_NoOverride(t *testing.T) {
	base := func(req *Request) (*Response, error) {
		if req.Headers["sec-ch-ua-platform"] != `"Custom"` {
			return nil, fmt.Errorf("expected custom platform, got %s", req.Headers["sec-ch-ua-platform"])
		}
		return &Response{StatusCode: 200}, nil
	}
	handler := ClientHintsMiddleware(base)
	_, err := handler(&Request{
		Method: "GET",
		URL:    "http://example.com",
		Headers: map[string]string{
			"user-agent":         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"sec-ch-ua-platform": `"Custom"`,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoggingMiddleware_Error(t *testing.T) {
	base := func(req *Request) (*Response, error) {
		return nil, fmt.Errorf("connection refused")
	}
	handler := LoggingMiddleware(base)
	_, err := handler(&Request{Method: "GET", URL: "http://example.com"})
	if err == nil {
		t.Fatal("expected error")
	}
}
