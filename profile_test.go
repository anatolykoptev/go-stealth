package stealth

import (
	"runtime"
	"testing"
)

func TestClientHintsHeaders_Chrome(t *testing.T) {
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	h := ClientHintsHeaders(ua)
	if h == nil {
		t.Fatal("expected hints for Chrome UA")
	}
	if h["sec-ch-ua-platform"] != `"Windows"` {
		t.Fatalf("unexpected platform: %s", h["sec-ch-ua-platform"])
	}
	if h["sec-ch-ua-mobile"] != "?0" {
		t.Fatalf("unexpected mobile: %s", h["sec-ch-ua-mobile"])
	}
}

func TestClientHintsHeaders_ChromeMobile(t *testing.T) {
	ua := "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36"
	h := ClientHintsHeaders(ua)
	if h == nil {
		t.Fatal("expected hints for mobile Chrome")
	}
	if h["sec-ch-ua-mobile"] != "?1" {
		t.Fatalf("expected mobile ?1, got %s", h["sec-ch-ua-mobile"])
	}
	if h["sec-ch-ua-platform"] != `"Android"` {
		t.Fatalf("expected Android platform, got %s", h["sec-ch-ua-platform"])
	}
}

func TestClientHintsHeaders_Edge(t *testing.T) {
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0"
	h := ClientHintsHeaders(ua)
	if h == nil {
		t.Fatal("expected hints for Edge UA")
	}
	if h["sec-ch-ua-mobile"] != "?0" {
		t.Fatalf("unexpected mobile: %s", h["sec-ch-ua-mobile"])
	}
	// Edge hints should include Microsoft Edge brand
	if h["sec-ch-ua"] == "" {
		t.Fatal("expected sec-ch-ua header")
	}
}

func TestClientHintsHeaders_Safari(t *testing.T) {
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15"
	h := ClientHintsHeaders(ua)
	if h != nil {
		t.Fatal("expected nil hints for Safari UA")
	}
}

func TestExtractChromeVersion(t *testing.T) {
	tests := []struct {
		ua      string
		version string
	}{
		{"...Chrome/131.0.0.0...", "131"},
		{"...Chrome/130.0...", "130"},
		{"Safari/605.1.15", "131"}, // fallback
	}
	for _, tt := range tests {
		v := ExtractChromeVersion(tt.ua)
		if v != tt.version {
			t.Fatalf("ExtractChromeVersion(%q) = %q, want %q", tt.ua, v, tt.version)
		}
	}
}

func TestRandomProfile_NoFilter(t *testing.T) {
	p := RandomProfile()
	if p.UserAgent == "" {
		t.Fatal("expected non-empty profile")
	}
	if p.Browser == "" || p.OS == "" {
		t.Fatalf("expected metadata, got browser=%q os=%q", p.Browser, p.OS)
	}
}

func TestRandomProfile_ByOS(t *testing.T) {
	for range 20 {
		p := RandomProfile(WithOS("windows"))
		if p.OS != "windows" {
			t.Fatalf("expected windows profile, got os=%q: %s", p.OS, p.UserAgent)
		}
	}
}

func TestRandomProfile_ByBrowser(t *testing.T) {
	for range 20 {
		p := RandomProfile(WithBrowser("firefox"))
		if p.Browser != "firefox" {
			t.Fatalf("expected firefox profile, got browser=%q: %s", p.Browser, p.UserAgent)
		}
	}
}

func TestRandomProfile_ByCombination(t *testing.T) {
	for range 20 {
		p := RandomProfile(WithOS("macos"), WithBrowser("chrome"))
		if p.OS != "macos" || p.Browser != "chrome" {
			t.Fatalf("expected macos+chrome, got os=%q browser=%q", p.OS, p.Browser)
		}
	}
}

func TestRandomProfile_MobileOnly(t *testing.T) {
	for range 20 {
		p := RandomProfile(WithMobile(true))
		if !p.Mobile {
			t.Fatalf("expected mobile profile, got: %s", p.UserAgent)
		}
	}
}

func TestRandomProfile_DesktopOnly(t *testing.T) {
	for range 20 {
		p := RandomProfile(WithMobile(false))
		if p.Mobile {
			t.Fatalf("expected desktop profile, got mobile: %s", p.UserAgent)
		}
	}
}

func TestRandomProfile_EdgeWindows(t *testing.T) {
	for range 20 {
		p := RandomProfile(WithBrowser("edge"), WithOS("windows"))
		if p.Browser != "edge" || p.OS != "windows" {
			t.Fatalf("expected edge+windows, got browser=%q os=%q", p.Browser, p.OS)
		}
	}
}

func TestRandomProfile_FallbackOnNoMatch(t *testing.T) {
	// Non-existent combo should fallback
	p := RandomProfile(WithOS("freebsd"))
	if p.UserAgent == "" {
		t.Fatal("expected fallback profile")
	}
}

func TestPlatformMatchedProfile(t *testing.T) {
	p := PlatformMatchedProfile()
	if p.UserAgent == "" {
		t.Fatal("expected non-empty UserAgent")
	}
	if p.Mobile {
		t.Fatal("PlatformMatchedProfile should return desktop")
	}

	switch runtime.GOOS {
	case "linux":
		if p.OS != "linux" {
			t.Fatalf("on linux, expected linux profile, got os=%q", p.OS)
		}
	case "darwin":
		if p.OS != "macos" {
			t.Fatalf("on darwin, expected macos profile, got os=%q", p.OS)
		}
	case "windows":
		if p.OS != "windows" {
			t.Fatalf("on windows, expected windows profile, got os=%q", p.OS)
		}
	}
}

func TestBuiltinProfiles_AllHaveMetadata(t *testing.T) {
	for i, p := range BuiltinProfiles {
		if p.Browser == "" {
			t.Errorf("profile %d missing Browser: %s", i, p.UserAgent)
		}
		if p.OS == "" {
			t.Errorf("profile %d missing OS: %s", i, p.UserAgent)
		}
		if p.UserAgent == "" {
			t.Errorf("profile %d has empty UserAgent", i)
		}
	}
}
