package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOxBrowserSolver_GetCookie_Empty(t *testing.T) {
	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: "http://unused"})
	if got := solver.GetCookie("example.com"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestOxBrowserSolver_Solve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "abc123"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL})
	challenge := &CloudflareError{Type: ChallengeJS, StatusCode: 503}
	cookie, err := solver.Solve("example.com", challenge)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "cf_clearance=abc123" {
		t.Errorf("expected cf_clearance=abc123, got %q", cookie)
	}

	// Should be cached now.
	if got := solver.GetCookie("example.com"); got != "cf_clearance=abc123" {
		t.Errorf("cache miss: got %q", got)
	}
}

func TestOxBrowserSolver_BlockNotSolvable(t *testing.T) {
	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: "http://unused"})
	challenge := &CloudflareError{Type: ChallengeBlock, StatusCode: 403}
	_, err := solver.Solve("example.com", challenge)
	if err == nil {
		t.Error("expected error for block challenge")
	}
}

func TestOxBrowserSolver_CacheTTL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "fresh"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{
		BaseURL:  srv.URL,
		CacheTTL: 1, // 1 nanosecond — expires immediately
	})
	challenge := &CloudflareError{Type: ChallengeJS, StatusCode: 503}
	_, err := solver.Solve("example.com", challenge)
	if err != nil {
		t.Fatal(err)
	}
	// Cache should have expired.
	if got := solver.GetCookie("example.com"); got != "" {
		t.Errorf("expected expired cache, got %q", got)
	}
}
