package stealth

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

var brandRe = regexp.MustCompile(`"([^"]+)";v="([^"]+)"`)

// parseBrands reads a sec-ch-ua value back into brand entries.
func parseBrands(s string) []UserAgentBrand {
	var out []UserAgentBrand
	for _, m := range brandRe.FindAllStringSubmatch(s, -1) {
		out = append(out, UserAgentBrand{m[1], m[2]})
	}
	return out
}

// The brand list equals the sec-ch-ua of the real Google Chrome 156 capture:
// GREASE brand, version and order.
func TestUserAgentData_Brands_EqualCapturedSecChUa(t *testing.T) {
	c := loadWire(t, "google_chrome_156_nav.json")
	id := capturedIdentity(t, c.HTTP2.Headers["user-agent"])
	got, err := id.UserAgentData("156.0.8078.80")
	if err != nil {
		t.Fatal(err)
	}
	want := parseBrands(c.HTTP2.Headers["sec-ch-ua"])
	if len(want) != 3 {
		t.Fatalf("capture sec-ch-ua parsed to %v", want)
	}
	if !slices.Equal(got.Brands, want) {
		t.Errorf("Brands\n got  %v\n want %v", got.Brands, want)
	}
	// Same brands in the same order in the full list; the real version on
	// Chromium and the browser brand, and the GREASE version with ".0.0.0".
	if len(got.FullVersionList) != 3 {
		t.Fatalf("FullVersionList = %v", got.FullVersionList)
	}
	for i, b := range got.FullVersionList {
		if b.Brand != want[i].Brand {
			t.Errorf("FullVersionList[%d].Brand = %q, want %q", i, b.Brand, want[i].Brand)
		}
		wantV := "156.0.8078.80"
		if strings.HasPrefix(b.Brand, "Not") {
			wantV = want[i].Version + ".0.0.0"
		}
		if b.Version != wantV {
			t.Errorf("FullVersionList[%d] = %v, want version %q", i, b, wantV)
		}
	}
	if got.Mobile || got.Platform != "macOS" {
		t.Errorf("Mobile/Platform = %v/%q, capture says ?0 and macOS", got.Mobile, got.Platform)
	}
	if mob := c.HTTP2.Headers["sec-ch-ua-mobile"]; mob != "?0" {
		t.Fatalf("capture changed: sec-ch-ua-mobile=%q", mob)
	}
}

// Known answer for Chrome 131, the value a real Chrome 131 sends and the one
// a hand-copied constant in a consumer got wrong (brand order, GREASE string).
func TestUserAgentData_Chrome131_KnownAnswer(t *testing.T) {
	bp, _ := profileForTLS(ProfileChrome131)
	id := BrowserIdentity{BrowserProfile: bp}
	got, err := id.UserAgentData("")
	if err != nil {
		t.Fatal(err)
	}
	want := []UserAgentBrand{{"Google Chrome", "131"}, {"Chromium", "131"}, {"Not_A Brand", "24"}}
	if !slices.Equal(got.Brands, want) {
		t.Errorf("Brands = %v, want %v", got.Brands, want)
	}
	if got.FullVersionList[0].Version != "131.0.0.0" {
		t.Errorf("reduced full version = %q", got.FullVersionList[0].Version)
	}
}

// The structured list and the header are one source: for every Chromium
// profile, Brands renders to the exact sec-ch-ua header.
func TestUserAgentData_AgreesWithHeaderForEveryProfile(t *testing.T) {
	seen := 0
	for _, set := range [][]BrowserProfile{BuiltinProfiles, legacyProfiles} {
		for _, bp := range set {
			if !strings.Contains(bp.UserAgent, "Chrome/") {
				continue
			}
			seen++
			id := BrowserIdentity{BrowserProfile: bp}
			ud, err := id.UserAgentData("")
			if err != nil {
				t.Fatalf("%s/%s: %v", bp.TLSProfile, bp.OS, err)
			}
			hdr := ClientHintsHeaders(bp.UserAgent)["sec-ch-ua"]
			if !slices.Equal(ud.Brands, parseBrands(hdr)) {
				t.Errorf("%s/%s: Brands %v != header %s", bp.TLSProfile, bp.OS, ud.Brands, hdr)
			}
			if ud.Mobile != (ClientHintsHeaders(bp.UserAgent)["sec-ch-ua-mobile"] == "?1") {
				t.Errorf("%s/%s: Mobile disagrees with header", bp.TLSProfile, bp.OS)
			}
			if bp.Browser == "edge" && !slices.ContainsFunc(ud.Brands, func(b UserAgentBrand) bool { return b.Brand == "Microsoft Edge" }) {
				t.Errorf("%s/%s: edge brand missing: %v", bp.TLSProfile, bp.OS, ud.Brands)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no Chromium profiles")
	}
}
