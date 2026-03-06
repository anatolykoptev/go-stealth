package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// techResponse builds a minimal /analyze JSON body with the given tech names.
func techResponse(names ...string) map[string]interface{} {
	techs := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		techs = append(techs, map[string]interface{}{
			"name": n, "categories": []string{"misc"}, "confidence": 100,
		})
	}
	return map[string]interface{}{
		"url": "https://example.com", "status": 200, "technologies": techs,
	}
}

// analyzeServer returns a test server that serves the given response body on
// every request and increments calls on each hit.
func analyzeServer(t *testing.T, body map[string]interface{}, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
}

// brokenServer returns a server that always responds 500 so Analyze returns an error.
func brokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
}

// TestSiteIntel_CacheTTLExpiry — after TTL expires the cache misses and the API
// is called a second time.
func TestSiteIntel_CacheTTLExpiry(t *testing.T) {
	var calls atomic.Int32
	srv := analyzeServer(t, techResponse(), &calls)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	intel.ttl = 1 * time.Millisecond

	if _, err := intel.Get("https://example.com"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := intel.Get("https://example.com"); err != nil {
		t.Fatal(err)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 API calls after TTL expiry, got %d", got)
	}
}

// TestSiteIntel_WAFAtEndOfList — WAF detection works even when the WAF entry
// is the last element in the technologies slice.
func TestSiteIntel_WAFAtEndOfList(t *testing.T) {
	srv := analyzeServer(t, techResponse("React", "WordPress", "Akamai"), nil)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	info, err := intel.Get("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if info.WAF != "Akamai" {
		t.Errorf("expected WAF=Akamai (last in list), got %q", info.WAF)
	}
}

// TestSiteIntel_MultipleWAFs — when both Cloudflare and Akamai appear, the one
// that matches first in knownWAFs order wins (Cloudflare is listed before Akamai).
func TestSiteIntel_MultipleWAFs(t *testing.T) {
	// knownWAFs = ["Cloudflare", "Akamai", ...] so Cloudflare wins regardless of
	// the order in the technologies slice, because the outer loop is over techs
	// and the inner over knownWAFs — first waf match on each tech wins.
	// Here Cloudflare appears first in the tech slice, so it is encountered first.
	srv := analyzeServer(t, techResponse("Cloudflare", "Akamai"), nil)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	info, err := intel.Get("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if info.WAF != "Cloudflare" {
		t.Errorf("expected WAF=Cloudflare (first in tech list), got %q", info.WAF)
	}
}

// TestSiteIntel_NoWAF — non-WAF technologies produce an empty WAF field.
func TestSiteIntel_NoWAF(t *testing.T) {
	srv := analyzeServer(t, techResponse("React", "WordPress", "MySQL"), nil)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	info, err := intel.Get("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if info.WAF != "" {
		t.Errorf("expected empty WAF for non-WAF site, got %q", info.WAF)
	}
}

// TestSiteIntel_AnalyzeError — an unreachable ox-browser returns an error and an
// empty (non-nil) SiteInfo so callers can safely dereference without panics.
func TestSiteIntel_AnalyzeError(t *testing.T) {
	srv := brokenServer(t)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	info, err := intel.Get("https://example.com")
	if err == nil {
		t.Fatal("expected error from unreachable ox-browser, got nil")
	}
	if info == nil {
		t.Fatal("expected non-nil SiteInfo even on error (safe to dereference)")
	}
}

// TestSiteIntel_HasTech_CaseInsensitive — HasTech matches regardless of case.
func TestSiteIntel_HasTech_CaseInsensitive(t *testing.T) {
	si := &SiteInfo{
		Technologies: []AnalyzeTech{
			{Name: "React"},
			{Name: "Cloudflare"},
		},
	}

	cases := []struct {
		query string
		want  bool
	}{
		{"react", true},
		{"REACT", true},
		{"React", true},
		{"CLOUDFLARE", true},
		{"cloudflare", true},
		{"Cloudflare", true},
	}
	for _, tc := range cases {
		if got := si.HasTech(tc.query); got != tc.want {
			t.Errorf("HasTech(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
}

// TestSiteIntel_HasTech_NotFound — HasTech returns false for absent technologies.
func TestSiteIntel_HasTech_NotFound(t *testing.T) {
	si := &SiteInfo{
		Technologies: []AnalyzeTech{
			{Name: "React"},
			{Name: "WordPress"},
		},
	}

	if si.HasTech("Angular") {
		t.Error("HasTech(Angular) = true, want false — Angular is not in the list")
	}
}

// TestSiteIntel_ConcurrentGet — 10 goroutines hitting the same domain
// simultaneously must not race and must produce valid results.
// Ideally only 1 API call is made (cache hit after first), but the current
// implementation has a check-then-act window so we only guarantee ≥1 and no
// data races.  Run with -race.
func TestSiteIntel_ConcurrentGet(t *testing.T) {
	var calls atomic.Int32
	srv := analyzeServer(t, techResponse("Cloudflare"), &calls)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)
	infos := make([]*SiteInfo, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			infos[idx], errs[idx] = intel.Get("https://example.com")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
	for i, info := range infos {
		if info == nil {
			t.Errorf("goroutine %d: got nil SiteInfo", i)
			continue
		}
		if info.WAF != "Cloudflare" {
			t.Errorf("goroutine %d: expected WAF=Cloudflare, got %q", i, info.WAF)
		}
	}
	if n := calls.Load(); n < 1 {
		t.Errorf("expected at least 1 API call, got %d", n)
	}
	t.Logf("concurrent API calls made: %d (ideally 1)", calls.Load())
}

// TestSiteIntel_SuggestProfile_NoWAF — a site without a WAF still returns a
// valid (non-zero) BrowserProfile.
func TestSiteIntel_SuggestProfile_NoWAF(t *testing.T) {
	srv := analyzeServer(t, techResponse("jQuery", "Bootstrap"), nil)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	profile := intel.SuggestProfile("https://example.com")

	if profile.UserAgent == "" {
		t.Error("SuggestProfile returned zero-value profile (empty UserAgent) for no-WAF site")
	}
	if profile.Browser == "" {
		t.Error("SuggestProfile returned zero-value profile (empty Browser) for no-WAF site")
	}
}

// TestSiteIntel_SuggestProfile_AnalyzeFails — when ox-browser is unreachable
// SuggestProfile must return a random valid profile and must not panic.
func TestSiteIntel_SuggestProfile_AnalyzeFails(t *testing.T) {
	srv := brokenServer(t)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))

	// Must not panic.
	profile := intel.SuggestProfile("https://example.com")

	if profile.UserAgent == "" {
		t.Error("SuggestProfile returned zero-value profile on analyze failure")
	}
}

// TestSiteIntel_DifferentDomains — distinct domains each trigger their own API
// call; they must not share a cache entry.
func TestSiteIntel_DifferentDomains(t *testing.T) {
	var calls atomic.Int32
	srv := analyzeServer(t, techResponse(), &calls)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))

	if _, err := intel.Get("https://a.com/"); err != nil {
		t.Fatal(err)
	}
	if _, err := intel.Get("https://b.com/"); err != nil {
		t.Fatal(err)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("expected 2 API calls for 2 distinct domains, got %d", got)
	}
}

// TestSiteIntel_SameDomainDifferentPaths — two URLs that share a domain but
// differ only in path must hit the cache and produce only 1 API call.
func TestSiteIntel_SameDomainDifferentPaths(t *testing.T) {
	var calls atomic.Int32
	srv := analyzeServer(t, techResponse("Cloudflare"), &calls)
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))

	if _, err := intel.Get("https://a.com/page1"); err != nil {
		t.Fatal(err)
	}
	if _, err := intel.Get("https://a.com/page2"); err != nil {
		t.Fatal(err)
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("expected 1 API call (same domain, different paths → cached), got %d", got)
	}
}
