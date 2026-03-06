package stealth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestOxBrowser_Solve_InvalidJSON — server returns malformed JSON → unmarshal error.
func TestOxBrowser_Solve_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	_, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err == nil {
		t.Fatal("expected JSON unmarshal error, got nil")
	}
}

// TestOxBrowser_Solve_HTTP500 — server returns 500 → error wraps HTTP status.
func TestOxBrowser_Solve_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	_, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err == nil {
		t.Fatal("expected HTTP 500 error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500, got: %v", err)
	}
}

// TestOxBrowser_Solve_EmptyCookies — status "ok" but cookies map is empty → returns empty map (not error).
func TestOxBrowser_Solve_EmptyCookies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{},
		})
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	cookies, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err != nil {
		t.Fatalf("expected no error for empty cookies, got: %v", err)
	}
	if len(cookies) != 0 {
		t.Errorf("expected empty map, got %v", cookies)
	}
}

// TestOxBrowser_Solve_StatusError — status "error" with message → error contains that message.
func TestOxBrowser_Solve_StatusError(t *testing.T) {
	const wantMsg = "challenge timed out"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "error",
			"error":  wantMsg,
		})
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	_, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err == nil {
		t.Fatal("expected error for status=error, got nil")
	}
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("error %q should contain %q", err.Error(), wantMsg)
	}
}

// TestOxBrowser_FetchSmart_ContextCanceled — canceled context before request → context error.
func TestOxBrowser_FetchSmart_ContextCanceled(t *testing.T) {
	// Server that blocks so cancellation is the only exit.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait until the client cancels.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before the call

	client := NewOxBrowserClient(srv.URL)
	_, err := client.FetchSmart(ctx, "https://example.com")
	if err == nil {
		t.Fatal("expected context canceled error, got nil")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("error should mention context, got: %v", err)
	}
}

// TestOxBrowser_Analyze_EmptyTechnologies — server returns empty tech array → valid response with 0 techs.
func TestOxBrowser_Analyze_EmptyTechnologies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"url":          "https://example.com",
			"status":       200,
			"technologies": []interface{}{},
		})
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	resp, err := client.Analyze(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(resp.Technologies) != 0 {
		t.Errorf("expected 0 technologies, got %d", len(resp.Technologies))
	}
}

// TestOxBrowser_Post_EmptyBody — server returns empty response body → JSON unmarshal error.
func TestOxBrowser_Post_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write nothing — empty 200 body.
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewOxBrowserClient(srv.URL)
	_, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err == nil {
		t.Fatal("expected JSON unmarshal error for empty body, got nil")
	}
}

// TestOxBrowser_Solve_SlowServer — server sleeps 2s, client has 1s timeout → timeout error.
func TestOxBrowser_Solve_SlowServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "late"},
		})
	}))
	defer srv.Close()

	// Build a client with a deliberately short timeout.
	client := &OxBrowserClient{
		baseURL: srv.URL,
		client:  &http.Client{Timeout: 100 * time.Millisecond},
	}

	_, err := client.Solve(context.Background(), "https://example.com", "js_challenge")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	// The error message varies by Go version; just confirm it is non-nil (checked above)
	// and contains a recognisable timeout hint.
	msg := err.Error()
	if !strings.Contains(msg, "timeout") && !strings.Contains(msg, "deadline") && !strings.Contains(msg, "context") {
		t.Errorf("expected timeout/deadline/context in error, got: %v", err)
	}
}
