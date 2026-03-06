package stealth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestSiteIntel_DetectsWAF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"url":    "https://example.com",
			"status": 200,
			"technologies": []map[string]interface{}{
				{"name": "Cloudflare", "categories": []string{"CDN"}, "confidence": 100},
				{"name": "React", "categories": []string{"JavaScript frameworks"}, "confidence": 100},
			},
		})
	}))
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	info, err := intel.Get("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if info.WAF != "Cloudflare" {
		t.Errorf("expected WAF=Cloudflare, got %q", info.WAF)
	}
	if !info.HasTech("React") {
		t.Error("expected React in tech stack")
	}
}

func TestSiteIntel_CachesResults(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"url": "https://example.com", "status": 200,
			"technologies": []map[string]interface{}{},
		})
	}))
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	_, _ = intel.Get("https://example.com")
	_, _ = intel.Get("https://example.com")
	if calls.Load() != 1 {
		t.Errorf("expected 1 API call (cached), got %d", calls.Load())
	}
}

func TestSiteIntel_SuggestProfile_Cloudflare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"url": "https://example.com", "status": 200,
			"technologies": []map[string]interface{}{
				{"name": "Cloudflare", "categories": []string{"CDN"}, "confidence": 100},
			},
		})
	}))
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	profile := intel.SuggestProfile("https://example.com")
	// Cloudflare sites should get Chrome profile (most common, least suspicious).
	if profile.Browser != "chrome" {
		t.Errorf("expected chrome for Cloudflare site, got %s", profile.Browser)
	}
}

func TestSiteIntel_DetectsAkamai(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"url": "https://example.com", "status": 200,
			"technologies": []map[string]interface{}{
				{"name": "Akamai", "categories": []string{"CDN"}, "confidence": 100},
			},
		})
	}))
	defer srv.Close()

	intel := NewSiteIntel(NewOxBrowserClient(srv.URL))
	info, _ := intel.Get("https://example.com")
	if info.WAF != "Akamai" {
		t.Errorf("expected WAF=Akamai, got %q", info.WAF)
	}
}
