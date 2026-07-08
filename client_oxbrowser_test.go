package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithOxBrowser_Integration(t *testing.T) {
	// Mock ox-browser /solve and /fetch-smart endpoints.
	oxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/solve" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "ok",
				"cookies": map[string]string{"cf_clearance": "integ-test"},
			})
			return
		}
		if r.URL.Path == "/fetch-smart" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": 200, "body": "<html>solved</html>",
				"method": "solved", "cf_detected": true, "elapsed_ms": 100,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer oxSrv.Close()

	client, err := NewClient(WithoutSSRFGuard(),
		WithStdHTTP(),
		WithOxBrowser(oxSrv.URL),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = client // Client created successfully with ox-browser integration.
}

func TestWithOxBrowser_WithExistingCookieSolver(t *testing.T) {
	// When WithCookieSolver is already set, WithOxBrowser should only add SmartFetchMiddleware.
	oxSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 200, "body": "<html>solved</html>",
			"method": "solved", "cf_detected": false, "elapsed_ms": 50,
		})
	}))
	defer oxSrv.Close()

	// Provide an explicit CookieProvider — ox-browser should not add its own solver.
	solver := NewOxBrowserSolver(OxBrowserSolverConfig{BaseURL: oxSrv.URL})
	client, err := NewClient(WithoutSSRFGuard(),
		WithStdHTTP(),
		WithCookieSolver(solver),
		WithOxBrowser(oxSrv.URL),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = client
}
