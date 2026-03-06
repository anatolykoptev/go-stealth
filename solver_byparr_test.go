package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestByparrSolver_GetCookie_Empty(t *testing.T) {
	t.Parallel()
	solver := NewByparrSolver(ByparrConfig{BaseURL: "http://localhost:0"})
	if got := solver.GetCookie("example.com"); got != "" {
		t.Errorf("GetCookie on empty cache = %q, want empty", got)
	}
}

func TestByparrSolver_GetCookie_Expired(t *testing.T) {
	t.Parallel()
	solver := NewByparrSolver(ByparrConfig{
		BaseURL:  "http://localhost:0",
		CacheTTL: time.Millisecond,
	})
	solver.mu.Lock()
	solver.cache["example.com"] = cachedCookie{
		cookie:    "cf_clearance=old",
		expiresAt: time.Now().Add(-time.Second),
	}
	solver.mu.Unlock()

	if got := solver.GetCookie("example.com"); got != "" {
		t.Errorf("GetCookie expired = %q, want empty", got)
	}
}

func TestByparrSolver_GetCookie_Valid(t *testing.T) {
	t.Parallel()
	solver := NewByparrSolver(ByparrConfig{
		BaseURL:  "http://localhost:0",
		CacheTTL: time.Hour,
	})
	solver.mu.Lock()
	solver.cache["example.com"] = cachedCookie{
		cookie:    "cf_clearance=fresh",
		expiresAt: time.Now().Add(time.Hour),
	}
	solver.mu.Unlock()

	if got := solver.GetCookie("example.com"); got != "cf_clearance=fresh" {
		t.Errorf("GetCookie = %q, want cf_clearance=fresh", got)
	}
}

func TestByparrSolver_Solve_BlockNotSolvable(t *testing.T) {
	t.Parallel()
	solver := NewByparrSolver(ByparrConfig{BaseURL: "http://localhost:0"})
	_, err := solver.Solve("example.com", &CloudflareError{Type: ChallengeBlock})
	if err == nil {
		t.Fatal("expected error for block challenge")
	}
}

func TestByparrSolver_Solve_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" {
			t.Errorf("path = %s, want /v1", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		var req solverRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Cmd != "request.get" {
			t.Errorf("cmd = %s, want request.get", req.Cmd)
		}
		if req.URL != "https://example.com" {
			t.Errorf("url = %s, want https://example.com", req.URL)
		}

		resp := solverResponse{
			Status: "ok",
			Solution: &solverSolution{
				URL:       "https://example.com",
				UserAgent: "Mozilla/5.0",
				Cookies: []solverCookie{
					{Name: "cf_clearance", Value: "test-token-abc"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	solver := NewByparrSolver(ByparrConfig{
		BaseURL:  srv.URL,
		CacheTTL: time.Hour,
	})

	cookie, err := solver.Solve("example.com", &CloudflareError{Type: ChallengeJS})
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if cookie != "cf_clearance=test-token-abc" {
		t.Errorf("cookie = %q, want cf_clearance=test-token-abc", cookie)
	}

	// Verify it was cached
	if got := solver.GetCookie("example.com"); got != "cf_clearance=test-token-abc" {
		t.Errorf("cached = %q, want cf_clearance=test-token-abc", got)
	}
}

func TestByparrSolver_Solve_NoClearanceCookie(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := solverResponse{
			Status: "ok",
			Solution: &solverSolution{
				Cookies: []solverCookie{
					{Name: "other_cookie", Value: "xyz"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	solver := NewByparrSolver(ByparrConfig{BaseURL: srv.URL})
	_, err := solver.Solve("example.com", &CloudflareError{Type: ChallengeJS})
	if err == nil {
		t.Fatal("expected error when cf_clearance not found")
	}
}

func TestByparrSolver_Solve_SolverError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := solverResponse{Status: "error", Message: "timeout"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	solver := NewByparrSolver(ByparrConfig{BaseURL: srv.URL})
	_, err := solver.Solve("example.com", &CloudflareError{Type: ChallengeJS})
	if err == nil {
		t.Fatal("expected error on solver error response")
	}
}

func TestByparrSolver_Solve_HTTP500(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	solver := NewByparrSolver(ByparrConfig{BaseURL: srv.URL})
	_, err := solver.Solve("example.com", &CloudflareError{Type: ChallengeJS})
	if err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}

func TestByparrSolver_Solve_CachesThenExpires(t *testing.T) {
	t.Parallel()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		resp := solverResponse{
			Status: "ok",
			Solution: &solverSolution{
				Cookies: []solverCookie{
					{Name: "cf_clearance", Value: "tok"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	solver := NewByparrSolver(ByparrConfig{
		BaseURL:  srv.URL,
		CacheTTL: 10 * time.Millisecond,
	})

	// First solve — caches
	_, err := solver.Solve("example.com", &CloudflareError{Type: ChallengeJS})
	if err != nil {
		t.Fatalf("first solve: %v", err)
	}
	if got := solver.GetCookie("example.com"); got == "" {
		t.Error("expected cached cookie after solve")
	}

	// Wait for TTL
	time.Sleep(15 * time.Millisecond)

	if got := solver.GetCookie("example.com"); got != "" {
		t.Errorf("expected expired cache, got %q", got)
	}
}

func TestByparrSolver_Solve_NilChallenge(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := solverResponse{
			Status: "ok",
			Solution: &solverSolution{
				Cookies: []solverCookie{
					{Name: "cf_clearance", Value: "nil-challenge-tok"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	solver := NewByparrSolver(ByparrConfig{BaseURL: srv.URL})
	cookie, err := solver.Solve("example.com", nil)
	if err != nil {
		t.Fatalf("Solve with nil challenge: %v", err)
	}
	if cookie != "cf_clearance=nil-challenge-tok" {
		t.Errorf("cookie = %q", cookie)
	}
}
