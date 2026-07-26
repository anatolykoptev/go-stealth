package stealth

import (
	"strings"
	"testing"
)

// TestBrowserClient_Identity_Default asserts that a bare NewClient() (no
// WithProfile / WithIdentity) exposes a self-consistent BrowserIdentity: the
// TLS profile is the pinned default (ProfileChrome131), the User-Agent is
// resolved from BuiltinProfiles, the Chrome major in the UA equals the major
// in the TLSProfile, and Client Hints are derived and carry the same major.
func TestBrowserClient_Identity_Default(t *testing.T) {
	c, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	id := c.Identity()
	if id.TLSProfile != ProfileChrome131 {
		t.Fatalf("default Identity().TLSProfile = %q, want %q", id.TLSProfile, ProfileChrome131)
	}
	if id.UserAgent == "" {
		t.Fatal("default Identity().UserAgent empty — must resolve from BuiltinProfiles")
	}
	uaMaj := ExtractChromeVersion(id.UserAgent)
	tlsMaj := profileMajor(id.TLSProfile)
	if uaMaj != tlsMaj {
		t.Fatalf("default Identity() UA major %q != TLSProfile major %q (UA=%s)", uaMaj, tlsMaj, id.UserAgent)
	}
	if id.ClientHints == nil {
		t.Fatal("default Identity().ClientHints nil — must be derived from the UA")
	}
	if !strings.Contains(id.ClientHints["sec-ch-ua"], `"Chromium";v="`+uaMaj+`"`) {
		t.Fatalf("default Identity() sec-ch-ua missing Chromium v=%q: %s", uaMaj, id.ClientHints["sec-ch-ua"])
	}
}

// TestBrowserClient_Identity_WithProfile asserts the WithProfile path
// resolves a matching UA + Client Hints for the chosen profile.
func TestBrowserClient_Identity_WithProfile(t *testing.T) {
	c, err := NewClient(WithProfile(ProfileChrome146), WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	id := c.Identity()
	if id.TLSProfile != ProfileChrome146 {
		t.Fatalf("WithProfile Identity().TLSProfile = %q, want %q", id.TLSProfile, ProfileChrome146)
	}
	if !strings.Contains(id.UserAgent, "Chrome/146") {
		t.Fatalf("WithProfile(Chrome146) Identity().UserAgent = %q, want Chrome/146", id.UserAgent)
	}
	if id.ClientHints == nil || !strings.Contains(id.ClientHints["sec-ch-ua"], `"Chromium";v="146"`) {
		t.Fatalf("WithProfile(Chrome146) ClientHints missing Chromium v=146: %v", id.ClientHints)
	}
}

// TestBrowserClient_Identity_WithIdentity asserts that an identity set via
// WithIdentity is returned verbatim by Identity() — the UA, profile, and
// hints the caller supplied are what the client reports as in use.
func TestBrowserClient_Identity_WithIdentity(t *testing.T) {
	bp := RandomProfile(WithBrowser("firefox"), WithOS("linux"))
	in := BrowserIdentity{
		BrowserProfile: bp,
	}
	c, err := NewClient(WithIdentity(in), WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	id := c.Identity()
	if id.UserAgent != bp.UserAgent {
		t.Fatalf("WithIdentity Identity().UserAgent = %q, want %q", id.UserAgent, bp.UserAgent)
	}
	if id.TLSProfile != bp.TLSProfile {
		t.Fatalf("WithIdentity Identity().TLSProfile = %q, want %q", id.TLSProfile, bp.TLSProfile)
	}
	if id.Browser != bp.Browser || id.OS != bp.OS {
		t.Fatalf("WithIdentity metadata drift: got browser=%q os=%q, want browser=%q os=%q", id.Browser, id.OS, bp.Browser, bp.OS)
	}
	// Firefox has no Client Hints — Identity() must report nil, not a stale map.
	if id.ClientHints != nil {
		t.Fatalf("WithIdentity(firefox) ClientHints = %v, want nil (Firefox sends no Client Hints)", id.ClientHints)
	}
}

// TestBrowserClient_Identity_WithIdentityDerivesHints asserts that when a
// caller supplies a BrowserIdentity without Client Hints for a Chromium UA,
// WithIdentity derives them so the identity the client reports is complete.
func TestBrowserClient_Identity_WithIdentityDerivesHints(t *testing.T) {
	bp := RandomProfile(WithBrowser("chrome"), WithOS("windows"))
	in := BrowserIdentity{BrowserProfile: bp}
	c, err := NewClient(WithIdentity(in), WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	id := c.Identity()
	if id.ClientHints == nil {
		t.Fatal("WithIdentity(chrome, no hints) should derive ClientHints, got nil")
	}
	maj := ExtractChromeVersion(bp.UserAgent)
	if !strings.Contains(id.ClientHints["sec-ch-ua"], `"Chromium";v="`+maj+`"`) {
		t.Fatalf("derived ClientHints missing Chromium v=%q: %s", maj, id.ClientHints["sec-ch-ua"])
	}
}

// TestUserAgentForProfile_ChromeMajors asserts that for every Chrome-family
// profile constant the package exports, UserAgentForProfile returns a UA
// whose Chrome major equals the profile's major. This is the lookup the 13
// consumer repos need to delete their hardcoded UA literals.
func TestUserAgentForProfile_ChromeMajors(t *testing.T) {
	chromeProfiles := []TLSProfile{
		ProfileChrome131, ProfileChrome133, ProfileChrome144, ProfileChrome146,
	}
	for _, p := range chromeProfiles {
		ua := UserAgentForProfile(p)
		if ua == "" {
			t.Errorf("UserAgentForProfile(%q) = \"\", want a Chrome UA", p)
			continue
		}
		uaMaj := ExtractChromeVersion(ua)
		tlsMaj := profileMajor(p)
		if uaMaj != tlsMaj {
			t.Errorf("UserAgentForProfile(%q): UA major %q != profile major %q (UA=%s)", p, uaMaj, tlsMaj, ua)
		}
	}
}

// TestUserAgentForProfile_NoEntry asserts the documented behaviour for a
// profile with no BuiltinProfiles entry: an empty string, not a fallback UA.
func TestUserAgentForProfile_NoEntry(t *testing.T) {
	if ua := UserAgentForProfile(TLSProfile("chrome_999_no_such")); ua != "" {
		t.Fatalf("UserAgentForProfile(unknown) = %q, want \"\" (documented no-entry behaviour)", ua)
	}
}
