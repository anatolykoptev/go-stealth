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

// TestRoundTrip_MultiSetCookie verifies that multiple Set-Cookie headers
// are preserved as separate values (not corrupted by joining on "; ",
// which is the separator between cookie ATTRIBUTES, not between cookies).
func TestRoundTrip_MultiSetCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "cf_clearance", Value: "xyz789", Path: "/"})
		w.WriteHeader(200)
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

	cookies := resp.Header["Set-Cookie"]
	if len(cookies) != 2 {
		t.Fatalf("expected 2 Set-Cookie headers, got %d: %v", len(cookies), cookies)
	}

	// Each cookie should be a complete cookie string, not a fragment
	foundSession := false
	foundCfClearance := false
	for _, c := range cookies {
		if strings.HasPrefix(c, "session=abc123") {
			foundSession = true
		}
		if strings.HasPrefix(c, "cf_clearance=xyz789") {
			foundCfClearance = true
		}
	}
	if !foundSession {
		t.Errorf("missing session cookie, got: %v", cookies)
	}
	if !foundCfClearance {
		t.Errorf("missing cf_clearance cookie, got: %v", cookies)
	}
}
