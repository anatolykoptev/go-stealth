package stealth

import (
	"slices"
	"strings"
	"testing"
)

// chrome_156 is Chrome 156's UA and Client Hints over tls-client's Chrome_152
// TLS profile. The reason that is honest: tls-client ships nothing newer, and
// the ClientHello, ja4_r, peetprint and HTTP/2 fingerprint of installed Google
// Chrome 156.0.8078.80 were measured identical to what Chrome_152 emits (see
// the live check in fingerprint_wire_live_test.go).

func TestChrome156_MapsToChrome152TLS(t *testing.T) {
	p156, err := mapTLSProfile(ProfileChrome156)
	if err != nil {
		t.Fatalf("chrome_156 has no backend mapping: %v", err)
	}
	p152, err := mapTLSProfile(ProfileChrome152)
	if err != nil {
		t.Fatal(err)
	}
	if p156.GetClientHelloStr() != p152.GetClientHelloStr() {
		t.Fatalf("chrome_156 ClientHello %q != chrome_152 %q", p156.GetClientHelloStr(), p152.GetClientHelloStr())
	}
}

// Same wire as chrome_152 apart from the per-connection extension shuffle.
func TestChrome156_WireMatchesChrome152(t *testing.T) {
	a := captureWire(t, WithProfile(ProfileChrome152))
	b := captureWire(t, WithProfile(ProfileChrome156))
	if !slices.Equal(a.Settings, b.Settings) || a.WindowUpdate != b.WindowUpdate || *a.Priority != *b.Priority {
		t.Fatalf("HTTP/2 preface differs: 152=%v/%d/%+v 156=%v/%d/%+v", a.Settings, a.WindowUpdate, *a.Priority, b.Settings, b.WindowUpdate, *b.Priority)
	}
	if !sameU16(a.Ciphers, b.Ciphers) {
		t.Fatalf("cipher suites differ: %#x vs %#x", a.Ciphers, b.Ciphers)
	}
	sa, sb := slices.Clone(a.Extensions), slices.Clone(b.Extensions)
	slices.Sort(sa)
	slices.Sort(sb)
	if !sameU16(sa, sb) {
		t.Fatalf("extension sets differ: %#x vs %#x", sa, sb)
	}
}

func TestChrome156_BuiltinVariantsAreCoherent(t *testing.T) {
	wantPlatform := map[string]string{"windows": `"Windows"`, "macos": `"macOS"`, "linux": `"Linux"`}
	uaOSToken := map[string]string{"windows": "Windows NT 10.0", "macos": "Macintosh", "linux": "X11; Linux"}
	got := map[string]bool{}
	for _, bp := range BuiltinProfiles {
		if bp.TLSProfile != ProfileChrome156 || bp.Browser != "chrome" {
			continue
		}
		got[bp.OS] = true
		if !strings.Contains(bp.UserAgent, "Chrome/156.0.0.0") {
			t.Errorf("%s: UA major is not 156: %s", bp.OS, bp.UserAgent)
		}
		if tok, ok := uaOSToken[bp.OS]; ok && !strings.Contains(bp.UserAgent, tok) {
			t.Errorf("%s: UA lacks OS token %q: %s", bp.OS, tok, bp.UserAgent)
		}
		h := ClientHintsHeaders(bp.UserAgent)
		if !strings.Contains(h["sec-ch-ua"], `"Chromium";v="156"`) || !strings.Contains(h["sec-ch-ua"], `"Google Chrome";v="156"`) {
			t.Errorf("%s: sec-ch-ua major is not 156: %s", bp.OS, h["sec-ch-ua"])
		}
		if want, ok := wantPlatform[bp.OS]; ok && h["sec-ch-ua-platform"] != want {
			t.Errorf("%s: sec-ch-ua-platform = %s, want %s", bp.OS, h["sec-ch-ua-platform"], want)
		}
	}
	for os := range wantPlatform {
		if !got[os] {
			t.Errorf("no chrome_156 BuiltinProfile for %s", os)
		}
	}
}

// 152 stays selectable for consumers that pin it, with its own UA.
func TestChrome156_Chrome152StaysResolvable(t *testing.T) {
	bc, err := NewClient(WithProfile(ProfileChrome152), WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bc.Close() })
	if id := bc.Identity(); id.TLSProfile != ProfileChrome152 || !strings.Contains(id.UserAgent, "Chrome/152.") {
		t.Fatalf("chrome_152 identity = %s / %s", id.TLSProfile, id.UserAgent)
	}
	if ua := UserAgentForProfile(ProfileChrome152); !strings.Contains(ua, "Chrome/152.") {
		t.Fatalf("UserAgentForProfile(chrome_152) = %q", ua)
	}
}

// The default client presents Chrome 156 end to end: fingerprint profile, UA,
// hints, and the package-level ChromeHeaders.
func TestChrome156_IsTheDefault(t *testing.T) {
	bc, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bc.Close() })
	id := bc.Identity()
	if id.TLSProfile != ProfileChrome156 || !strings.Contains(id.UserAgent, "Chrome/156.") {
		t.Fatalf("default identity = %s / %s, want chrome_156 / Chrome/156", id.TLSProfile, id.UserAgent)
	}
	for range 20 {
		if ua := ChromeHeaders()["user-agent"]; !strings.Contains(ua, "Chrome/156.") {
			t.Fatalf("ChromeHeaders() UA = %s, want Chrome/156", ua)
		}
	}
}
