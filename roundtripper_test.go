package stealth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoundTrip_GET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}

	stdClient := client.StdClient()

	resp, err := stdClient.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", body)
	}

	if resp.Header.Get("X-Custom") != "value" {
		t.Fatalf("expected X-Custom header, got: %v", resp.Header)
	}
}

func TestRoundTrip_POST(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Write(b)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}

	stdClient := client.StdClient()
	resp, err := stdClient.Post(server.URL, "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestRoundTrip_WithMiddleware(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Injected") != "yes" {
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

	// Middleware should be applied even through RoundTripper
	client.Use(func(next Handler) Handler {
		return func(req *Request) (*Response, error) {
			if req.Headers == nil {
				req.Headers = make(map[string]string)
			}
			req.Headers["X-Injected"] = "yes"
			return next(req)
		}
	})

	stdClient := client.StdClient()
	resp, err := stdClient.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRoundTrip_ImplementsInterface(t *testing.T) {
	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	var _ http.RoundTripper = client
}

func TestStdClient_NotNil(t *testing.T) {
	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	std := client.StdClient()
	if std == nil {
		t.Fatal("StdClient returned nil")
	}
	if std.Transport != client {
		t.Fatal("Transport should be the BrowserClient")
	}
}
