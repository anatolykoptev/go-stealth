package stealth

import (
	"regexp"
	"strings"
	"testing"
)

// firstNumericToken returns the first all-digit "_"-separated token of a TLS
// profile name: chrome_146 -> 146, safari_16_0 -> 16, safari_ios_26_0 -> 26.
func firstNumericToken(p TLSProfile) string {
	for _, tok := range strings.Split(string(p), "_") {
		if tok != "" && strings.Trim(tok, "0123456789") == "" {
			return tok
		}
	}
	return ""
}

func allProfiles() []BrowserProfile {
	return append(append([]BrowserProfile{}, BuiltinProfiles...), legacyProfiles...)
}

var (
	reFirefoxVer = regexp.MustCompile(`Firefox/(\d+)\.`)
	reFirefoxRV  = regexp.MustCompile(`rv:(\d+)\.`)
	reSafariVer  = regexp.MustCompile(`Version/(\d+)\.`)
	reEdgeVer    = regexp.MustCompile(`Edg/(\d+)\.`)
)

// TestBuiltinProfilesCoherent pins, for every profile (current and legacy),
// that the TLS profile, User-Agent, platform fields and Client Hints describe
// the same browser: family and major of the TLS profile equal those of the UA;
// Chromium-family entries carry hints that agree with the UA; Firefox and
// Safari carry none.
func TestBuiltinProfilesCoherent(t *testing.T) {
	for _, bp := range allProfiles() {
		name := string(bp.TLSProfile) + "/" + bp.Browser + "/" + bp.OS
		t.Run(name, func(t *testing.T) {
			ua, major := bp.UserAgent, firstNumericToken(bp.TLSProfile)
			if major == "" {
				t.Fatalf("cannot read major from TLS profile %q", bp.TLSProfile)
			}
			hints := ClientHintsHeaders(ua)

			switch bp.Browser {
			case "chrome", "edge", "brave":
				wantFamily := map[string]string{"chrome": "chrome_", "edge": "chrome_", "brave": "brave_"}[bp.Browser]
				if !strings.HasPrefix(string(bp.TLSProfile), wantFamily) {
					t.Errorf("%s entry rides TLS profile %q, want family %q", bp.Browser, bp.TLSProfile, wantFamily)
				}
				if got := ExtractChromeVersion(ua); got != major {
					t.Errorf("UA Chrome major %q != TLS profile major %q", got, major)
				}
				if strings.Contains(ua, "Firefox/") || strings.Contains(ua, "Version/") {
					t.Errorf("Chromium UA carries Firefox/Safari tokens: %s", ua)
				}
				if bp.Browser == "edge" {
					if m := reEdgeVer.FindStringSubmatch(ua); m == nil || m[1] != major {
						t.Errorf("Edge UA major != TLS major %q: %s", major, ua)
					}
				}
				if hints == nil {
					t.Fatal("Chromium-family entry has no client hints")
				}
				chUa := hints["sec-ch-ua"]
				if !strings.Contains(chUa, `"Chromium";v="`+major+`"`) {
					t.Errorf("sec-ch-ua lacks Chromium v=%s: %s", major, chUa)
				}
				brand := `"Google Chrome";v="` + major + `"`
				if bp.Browser == "edge" {
					brand = `"Microsoft Edge";v="` + major + `"`
				}
				if !strings.Contains(chUa, brand) {
					t.Errorf("sec-ch-ua lacks %s: %s", brand, chUa)
				}
				wantPlatform := map[string]string{"windows": `"Windows"`, "macos": `"macOS"`, "linux": `"Linux"`, "android": `"Android"`}[bp.OS]
				if hints["sec-ch-ua-platform"] != wantPlatform {
					t.Errorf("sec-ch-ua-platform = %s, want %s", hints["sec-ch-ua-platform"], wantPlatform)
				}
			case "firefox":
				if !strings.HasPrefix(string(bp.TLSProfile), "firefox_") {
					t.Errorf("firefox entry rides TLS profile %q", bp.TLSProfile)
				}
				m1, m2 := reFirefoxVer.FindStringSubmatch(ua), reFirefoxRV.FindStringSubmatch(ua)
				if m1 == nil || m2 == nil || m1[1] != major || m2[1] != major {
					t.Errorf("Firefox UA major (Firefox/ and rv:) != TLS major %q: %s", major, ua)
				}
				if strings.Contains(ua, "Chrome/") || hints != nil {
					t.Errorf("Firefox entry carries Chrome identity (UA=%s hints=%v)", ua, hints)
				}
			case "safari":
				if !strings.HasPrefix(string(bp.TLSProfile), "safari_") {
					t.Errorf("safari entry rides TLS profile %q", bp.TLSProfile)
				}
				if m := reSafariVer.FindStringSubmatch(ua); m == nil || m[1] != major {
					t.Errorf("Safari UA Version major != TLS major %q: %s", major, ua)
				}
				if strings.Contains(ua, "Chrome/") || hints != nil {
					t.Errorf("Safari entry carries Chrome identity (UA=%s hints=%v)", ua, hints)
				}
			default:
				t.Fatalf("unknown browser %q", bp.Browser)
			}

			osToken := map[string]string{"windows": "Windows NT", "macos": "Macintosh", "linux": "X11", "android": "Android", "ios": "iPhone"}[bp.OS]
			if !strings.Contains(ua, osToken) {
				t.Errorf("OS %q but UA lacks %q: %s", bp.OS, osToken, ua)
			}
			if bp.Mobile != strings.Contains(ua, "Mobile") {
				t.Errorf("Mobile=%v disagrees with UA: %s", bp.Mobile, ua)
			}
			if bp.OS == "android" && !strings.Contains(ua, "Android 10; K") {
				t.Errorf("Android Chrome sends the reduced UA platform 'Android 10; K': %s", ua)
			}
			if hints != nil && hints["sec-ch-ua-mobile"] != map[bool]string{true: "?1", false: "?0"}[bp.Mobile] {
				t.Errorf("sec-ch-ua-mobile %s disagrees with Mobile=%v", hints["sec-ch-ua-mobile"], bp.Mobile)
			}
			if _, ok := profileMap[bp.TLSProfile]; !ok {
				t.Errorf("TLS profile %q is not mapped to a backend profile", bp.TLSProfile)
			}
		})
	}
}

// TestEveryMappedProfileHasIdentity: a profile the backend can build must also
// resolve to a User-Agent, otherwise Identity() would present a TLS profile
// with an empty UA.
func TestEveryMappedProfileHasIdentity(t *testing.T) {
	for p := range profileMap {
		if bp, ok := profileForTLS(p); !ok || bp.UserAgent == "" {
			t.Errorf("mapped TLS profile %q has no identity entry", p)
		}
	}
}

// TestClientHintsMatchRealBrowsers compares ClientHintsHeaders with sec-ch-ua
// strings captured from real Chromium builds (fixtures are literal captures,
// not the output of the code under test): the GREASE brand, its version and
// the brand order all change with the major.
func TestClientHintsMatchRealBrowsers(t *testing.T) {
	tests := []struct {
		name, ua, want string
	}{
		{"chrome145", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
			`"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`},
		{"chrome156", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36",
			`"Not:A-Brand";v="8", "Chromium";v="156", "Google Chrome";v="156"`},
	}
	for _, tt := range tests {
		if got := ClientHintsHeaders(tt.ua)["sec-ch-ua"]; got != tt.want {
			t.Errorf("%s: sec-ch-ua = %s, want %s", tt.name, got, tt.want)
		}
	}
}

// TestDefaultHeaderOrderMatchesBrowser: a Firefox client must not send the
// Chrome header order (and vice versa).
func TestDefaultHeaderOrderMatchesBrowser(t *testing.T) {
	ff, err := NewClient(WithProfile(ProfileFirefox148), WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	if got := ff.headerOrder[0]; got != "user-agent" {
		t.Errorf("firefox client header order starts with %q, want user-agent", got)
	}
	ch, err := NewClient(WithProfile(ProfileChrome152), WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	if got := ch.headerOrder[0]; got != "sec-ch-ua" {
		t.Errorf("chrome client header order starts with %q, want sec-ch-ua", got)
	}
}
