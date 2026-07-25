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

// profileMajor extracts the numeric major from a TLSProfile constant of the
// form "chrome_<N>" / "firefox_<N>". Returns "" for profiles without a single
// numeric major (safari uses "safari_16_0" etc. — not covered by this check).
func profileMajor(p TLSProfile) string {
	s := string(p)
	idx := strings.IndexByte(s, '_')
	if idx == -1 {
		return ""
	}
	rest := s[idx+1:]
	// keep only the leading numeric run (chrome_146 -> 146; chrome_131_PSK -> 131)
	var b strings.Builder
	for _, r := range rest {
		if r < '0' || r > '9' {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestBuiltinProfiles_ChromeUAMatchesTLSProfile is the whole-table invariant:
// for every Chrome (and Edge, which rides a Chrome TLS profile) entry, the
// Chrome major encoded in the User-Agent MUST equal the major encoded in the
// TLSProfile constant. A mismatch (UA says 146, JA3 says 131) is an active
// bot signal — the whole point of this package.
func TestBuiltinProfiles_ChromeUAMatchesTLSProfile(t *testing.T) {
	for i, p := range BuiltinProfiles {
		if p.Browser != "chrome" && p.Browser != "edge" {
			continue
		}
		uaMajor := ExtractChromeVersion(p.UserAgent)
		tlsMajor := profileMajor(p.TLSProfile)
		if tlsMajor == "" {
			t.Errorf("profile %d (%s): could not parse major from TLSProfile %q", i, p.Browser, p.TLSProfile)
			continue
		}
		if uaMajor != tlsMajor {
			t.Errorf("profile %d (%s/%s): UA major %q != TLSProfile major %q (UA=%s)",
				i, p.Browser, p.OS, uaMajor, tlsMajor, p.UserAgent)
		}
	}
}

// TestClientHintsHeaders_ChromeBrandConsistency asserts that for a Chrome UA,
// sec-ch-ua carries THREE brands including "Google Chrome", and the Chromium
// brand version equals the UA's Chrome major. The old two-brand implementation
// (which omitted "Google Chrome") must fail this test.
func TestClientHintsHeaders_ChromeBrandConsistency(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		maj  string
	}{
		{"chrome146", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36", "146"},
		{"chrome144", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36", "144"},
		{"chrome131", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36", "131"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := ClientHintsHeaders(tt.ua)
			if h == nil {
				t.Fatal("expected hints for Chrome UA")
			}
			chUa := h["sec-ch-ua"]
			// Must include the "Google Chrome" brand — the old impl omitted it.
			if !strings.Contains(chUa, `"Google Chrome";v="`+tt.maj+`"`) {
				t.Errorf("sec-ch-ua missing Google Chrome brand v=%q: %s", tt.maj, chUa)
			}
			// Chromium brand version must equal the UA major.
			wantChromium := `"Chromium";v="` + tt.maj + `"`
			if !strings.Contains(chUa, wantChromium) {
				t.Errorf("sec-ch-ua missing/incorrect Chromium brand: want %s in %s", wantChromium, chUa)
			}
			// Must carry a GREASE brand (a "Not*Brand" entry).
			if !strings.Contains(chUa, `"Not`) || !strings.Contains(chUa, `Brand"`) {
				t.Errorf("sec-ch-ua missing GREASE (Not*Brand) token: %s", chUa)
			}
		})
	}
}
