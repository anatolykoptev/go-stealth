package stealth

import (
	"runtime"
	"strings"
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

func TestPlatformMatchedProfile(t *testing.T) {
	p := PlatformMatchedProfile()
	if p.UserAgent == "" {
		t.Fatal("expected non-empty UserAgent")
	}

	// On Linux, should return a Linux/X11 profile
	if runtime.GOOS == "linux" {
		if !strings.Contains(p.UserAgent, "Linux") && !strings.Contains(p.UserAgent, "X11") {
			t.Fatalf("on Linux, expected Linux UA, got: %s", p.UserAgent)
		}
	}
}

func TestMatchesOS(t *testing.T) {
	tests := []struct {
		ua   string
		goos string
		want bool
	}{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "windows", true},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64)", "linux", false},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", "darwin", true},
		{"Mozilla/5.0 (X11; Linux x86_64)", "linux", true},
		{"Mozilla/5.0 (X11; Ubuntu; Linux x86_64)", "linux", true},
	}
	for _, tt := range tests {
		got := matchesOS(tt.ua, tt.goos)
		if got != tt.want {
			t.Errorf("matchesOS(%q, %q) = %v, want %v", tt.ua, tt.goos, got, tt.want)
		}
	}
}
