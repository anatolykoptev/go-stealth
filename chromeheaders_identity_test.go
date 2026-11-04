package stealth

import (
	"strings"
	"testing"
)

// TestChromeHeadersFor_MatchesProfileMajor asserts that for every current
// Chromium-based BuiltinProfile the profile-matched headers carry the profile's
// own User-Agent and a sec-ch-ua whose Chromium major equals the TLS profile
// major. Pairing a Chrome 146 fingerprint with Chrome/152 headers (the pool
// UA) is the mismatch this guards.
func TestChromeHeadersFor_MatchesProfileMajor(t *testing.T) {
	seen := 0
	for _, bp := range BuiltinProfiles {
		if bp.Browser != "chrome" && bp.Browser != "edge" {
			continue
		}
		seen++
		h := ChromeHeadersFor(bp)
		tlsMaj := profileMajor(bp.TLSProfile)
		if got := ExtractChromeVersion(h["user-agent"]); got != tlsMaj {
			t.Errorf("%s/%s: UA major %q != TLS profile major %q (UA=%s)", bp.TLSProfile, bp.OS, got, tlsMaj, h["user-agent"])
		}
		if h["user-agent"] != bp.UserAgent {
			t.Errorf("%s/%s: user-agent = %q, want the profile UA %q", bp.TLSProfile, bp.OS, h["user-agent"], bp.UserAgent)
		}
		if !strings.Contains(h["sec-ch-ua"], `"Chromium";v="`+tlsMaj+`"`) {
			t.Errorf("%s/%s: sec-ch-ua lacks Chromium v=%s: %s", bp.TLSProfile, bp.OS, tlsMaj, h["sec-ch-ua"])
		}
		if bp.Browser == "edge" && !strings.Contains(h["sec-ch-ua"], `"Microsoft Edge"`) {
			t.Errorf("%s/%s: edge profile sec-ch-ua lacks Microsoft Edge brand: %s", bp.TLSProfile, bp.OS, h["sec-ch-ua"])
		}
		if h["accept-encoding"] == "" || h["sec-fetch-dest"] != "document" {
			t.Errorf("%s/%s: base navigation headers missing: %v", bp.TLSProfile, bp.OS, h)
		}
	}
	if seen == 0 {
		t.Fatal("no Chrome/Edge BuiltinProfiles found")
	}
}

// TestChromeHeadersFor_NonChromiumIsNil: Chrome-shaped headers over a
// Firefox/Safari profile would themselves be a mismatch.
func TestChromeHeadersFor_NonChromiumIsNil(t *testing.T) {
	for _, bp := range BuiltinProfiles {
		if bp.Browser == "firefox" || bp.Browser == "safari" {
			if h := ChromeHeadersFor(bp); h != nil {
				t.Errorf("%s: ChromeHeadersFor = %v, want nil", bp.TLSProfile, h)
			}
		}
	}
}

// TestBrowserClient_ChromeHeaders_FollowsClientIdentity: the client method
// returns headers for the profile the client actually presents.
func TestBrowserClient_ChromeHeaders_FollowsClientIdentity(t *testing.T) {
	for _, p := range []TLSProfile{ProfileChrome146, ProfileChrome150, ProfileChrome152} {
		c, err := NewClient(WithProfile(p), WithoutSSRFGuard())
		if err != nil {
			t.Fatal(err)
		}
		h := c.ChromeHeaders()
		maj := profileMajor(p)
		if got := ExtractChromeVersion(h["user-agent"]); got != maj {
			t.Errorf("%s: client ChromeHeaders UA major %q, want %q", p, got, maj)
		}
		if h["user-agent"] != c.Identity().UserAgent {
			t.Errorf("%s: client ChromeHeaders UA %q != Identity().UserAgent %q", p, h["user-agent"], c.Identity().UserAgent)
		}
		if !strings.Contains(h["sec-ch-ua"], `"Chromium";v="`+maj+`"`) {
			t.Errorf("%s: sec-ch-ua lacks Chromium v=%s: %s", p, maj, h["sec-ch-ua"])
		}
	}
}

// TestChromeHeaders_MatchesDefaultClientIdentity: the package-level
// ChromeHeaders describes the default client, so its UA major must equal the
// default client's TLS profile major on every draw.
func TestChromeHeaders_MatchesDefaultClientIdentity(t *testing.T) {
	c, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	maj := profileMajor(c.Identity().TLSProfile)
	for range 300 {
		ua := ChromeHeaders()["user-agent"]
		if got := ExtractChromeVersion(ua); got != maj {
			t.Fatalf("ChromeHeaders UA major %q != default client TLS major %q (UA=%s)", got, maj, ua)
		}
	}
	for _, ua := range defaultChromeUserAgents() {
		if got := ExtractChromeVersion(ua); got != maj {
			t.Fatalf("default Chrome UA pool holds major %q, default client is %q (UA=%s)", got, maj, ua)
		}
	}
}

// TestRandomUserAgents_ChromeEntriesMatchDefault: the exported pool's Chrome
// entries describe the default profile only.
func TestRandomUserAgents_ChromeEntriesMatchDefault(t *testing.T) {
	maj := profileMajor(defaultConfig().profile)
	for _, ua := range RandomUserAgents {
		if strings.Contains(ua, "Chrome/") && ExtractChromeVersion(ua) != maj {
			t.Errorf("RandomUserAgents Chrome entry major != default %s: %s", maj, ua)
		}
	}
}
