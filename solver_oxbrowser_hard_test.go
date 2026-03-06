package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestOxBrowserSolver_NilChallenge — Solve(domain, nil) defaults to "js_challenge".
func TestOxBrowserSolver_NilChallenge(t *testing.T) {
	var gotChallengeType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			gotChallengeType = req["challenge_type"]
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "niltest"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL})
	cookie, err := solver.Solve("example.com", nil)
	if err != nil {
		t.Fatalf("expected no error for nil challenge, got: %v", err)
	}
	if cookie != "cf_clearance=niltest" {
		t.Errorf("unexpected cookie: %q", cookie)
	}
	if gotChallengeType != "js_challenge" {
		t.Errorf("expected challenge_type=js_challenge, got %q", gotChallengeType)
	}
}

// TestOxBrowserSolver_EmptyDomain — Solve("", challenge) constructs "https://" URL; server sees it.
func TestOxBrowserSolver_EmptyDomain(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			gotURL = req["url"]
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "empty-domain"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL})
	challenge := &CloudflareError{Type: ChallengeJS, StatusCode: 503}
	_, err := solver.Solve("", challenge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != "https://" {
		t.Errorf("expected url=\"https://\", got %q", gotURL)
	}
}

// TestOxBrowserSolver_NoCfClearanceInResponse — cookies lack cf_clearance → specific error.
func TestOxBrowserSolver_NoCfClearanceInResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"some_other_cookie": "value"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL})
	_, err := solver.Solve("example.com", nil)
	if err == nil {
		t.Fatal("expected error when cf_clearance absent, got nil")
	}
	if !strings.Contains(err.Error(), "cf_clearance") {
		t.Errorf("error should mention cf_clearance, got: %v", err)
	}
}

// TestOxBrowserSolver_DefaultTTL — config with CacheTTL=0 uses 25-minute default.
// Verified indirectly: cache entry survives 10ms but expires immediately with 1ns TTL.
func TestOxBrowserSolver_DefaultTTL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "default-ttl"},
		})
	}))
	defer srv.Close()

	// CacheTTL=0 → should default to 25 minutes, not expire instantly.
	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL, CacheTTL: 0})
	if solver.ttl != 25*time.Minute {
		t.Errorf("expected default TTL 25m, got %v", solver.ttl)
	}

	_, err := solver.Solve("example.com", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tiny sleep then confirm cache is still live (25min hasn't elapsed).
	time.Sleep(10 * time.Millisecond)
	if got := solver.GetCookie("example.com"); got != "cf_clearance=default-ttl" {
		t.Errorf("expected cached cookie, got %q", got)
	}
}

// TestOxBrowserSolver_CacheConcurrent — 10 goroutines calling Solve + GetCookie simultaneously.
// Run with -race to detect data races.
func TestOxBrowserSolver_CacheConcurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "concurrent"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL})

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()
			_, _ = solver.Solve("example.com", nil)
			_ = solver.GetCookie("example.com")
		}()
	}
	wg.Wait()

	// At least one Solve should have cached the cookie.
	if got := solver.GetCookie("example.com"); got == "" {
		t.Error("expected cached cookie after concurrent solves, got empty string")
	}
}

// TestOxBrowserSolver_CacheOverwrite — solve domain A twice → cache has second value.
func TestOxBrowserSolver_CacheOverwrite(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		val := "first"
		if callCount > 1 {
			val = "second"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": val},
		})
	}))
	defer srv.Close()

	// Use 1ns TTL so that the first entry expires before the second call.
	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL, CacheTTL: 1})

	_, err := solver.Solve("example.com", nil)
	if err != nil {
		t.Fatalf("first solve error: %v", err)
	}
	// Wait for first entry to expire.
	time.Sleep(time.Millisecond)

	_, err = solver.Solve("example.com", nil)
	if err != nil {
		t.Fatalf("second solve error: %v", err)
	}

	// Bump TTL on the solver so the second entry stays long enough to read.
	solver.ttl = time.Minute
	// Force a third solve so the long-TTL entry enters the cache.
	_, err = solver.Solve("example.com", nil)
	if err != nil {
		t.Fatalf("third solve error: %v", err)
	}

	got := solver.GetCookie("example.com")
	if got != "cf_clearance=second" {
		t.Errorf("expected overwritten cache with second value, got %q", got)
	}
}

// TestOxBrowserSolver_ManagedAt200Challenge — ChallengeManagedAt200 passes correct challenge_type.
func TestOxBrowserSolver_ManagedAt200Challenge(t *testing.T) {
	var gotChallengeType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			gotChallengeType = req["challenge_type"]
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"cookies": map[string]string{"cf_clearance": "managed200"},
		})
	}))
	defer srv.Close()

	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: srv.URL})
	challenge := &CloudflareError{Type: ChallengeManagedAt200, StatusCode: 200}

	cookie, err := solver.Solve("example.com", challenge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cookie != "cf_clearance=managed200" {
		t.Errorf("unexpected cookie: %q", cookie)
	}
	if gotChallengeType != string(ChallengeManagedAt200) {
		t.Errorf("expected challenge_type=%q, got %q", string(ChallengeManagedAt200), gotChallengeType)
	}
}
