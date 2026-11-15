package stealth

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/bogdanfinn/tls-client/profiles"
)

// Microsoft Edge 156 is measured, not derived: the oracle is the captures in
// testdata/wire (Edge 156.0.4314.58 on Windows and Linux, through the same
// echo service as the Chrome 156 captures). Expected values are read from those
// files, never retyped here. Edge shares Chromium 156.0.8078.80 with Chrome but
// differs on the wire in the ClientHello (no trust_anchors extension) and in the
// navigation Accept (no image/jxl).

// ja4r is the parsed ja4_r of an echo capture: cipher suites and extensions
// sorted, SNI and ALPN left out, signature algorithms in wire order, GREASE
// removed.
type ja4r struct{ ciphers, exts, sigalgs []uint16 }

func parseHexList(t *testing.T, s string) []uint16 {
	t.Helper()
	var out []uint16
	for _, f := range strings.Split(s, ",") {
		v, err := strconv.ParseUint(f, 16, 16)
		if err != nil {
			t.Fatalf("ja4_r field %q: %v", f, err)
		}
		out = append(out, uint16(v))
	}
	return out
}

func parseJA4R(t *testing.T, s string) ja4r {
	t.Helper()
	p := strings.Split(s, "_")
	if len(p) != 4 {
		t.Fatalf("ja4_r %q has %d parts, want 4", s, len(p))
	}
	return ja4r{parseHexList(t, p[1]), parseHexList(t, p[2]), parseHexList(t, p[3])}
}

// wireJA4R derives the same view from a ClientHello this package sent to the
// local listener.
func wireJA4R(w wireCapture) ja4r {
	var r ja4r
	for _, c := range w.Ciphers {
		if c != greaseNormalised {
			r.ciphers = append(r.ciphers, c)
		}
	}
	for _, e := range w.Extensions {
		if e != greaseNormalised && e != 0x0000 && e != 0x0010 {
			r.exts = append(r.exts, e)
		}
	}
	d := w.ExtData[0x000d]
	for i := 2; i+1 < len(d); i += 2 {
		if v := binary.BigEndian.Uint16(d[i:]); !isGrease(v) {
			r.sigalgs = append(r.sigalgs, v)
		}
	}
	slices.Sort(r.ciphers)
	slices.Sort(r.exts)
	return r
}

func (r ja4r) String() string {
	hex := func(v []uint16) string {
		s := make([]string, len(v))
		for i, x := range v {
			s[i] = fmt.Sprintf("%04x", x)
		}
		return strings.Join(s, ",")
	}
	return hex(r.ciphers) + "_" + hex(r.exts) + "_" + hex(r.sigalgs)
}

type edgeWireFile struct {
	TLS struct {
		JA4R string `json:"ja4_r"`
	} `json:"tls"`
	HTTP2 struct {
		Akamai   string `json:"akamai_fingerprint"`
		Priority struct {
			Weight int `json:"weight"`
		} `json:"priority"`
	} `json:"http2"`
}

func loadEdgeWire(t *testing.T, name string) edgeWireFile {
	t.Helper()
	b, err := os.ReadFile("testdata/wire/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var f edgeWireFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// The Chrome 156 control proves the parser: the same derivation applied to what
// chrome_156 puts on the wire equals the real Chrome capture's ja4_r. The Edge
// cases then use that same derivation.
func TestEdge156_ClientHelloEqualsCapture(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []ClientOption
		file string
	}{
		{"chrome_156 control", []ClientOption{WithProfile(ProfileChrome156)}, "google_chrome_156_nav.json"},
		{"edge_156 windows", []ClientOption{WithProfile(ProfileEdge156)}, "microsoft_edge_156_windows_nav.json"},
		{"edge_156 linux", []ClientOption{WithProfile(ProfileEdge156)}, "microsoft_edge_156_linux_nav.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := loadEdgeWire(t, tc.file)
			want := parseJA4R(t, ref.TLS.JA4R)
			// The extension order is shuffled per connection; ja4_r sorts it.
			for range 3 {
				got := wireJA4R(captureWire(t, tc.opts...))
				if got.String() != want.String() {
					t.Fatalf("ja4_r\n got  %s\n want %s", got, want)
				}
			}
		})
	}
}

// edge_156 is chrome_156 minus one extension, and the real captures differ by
// the same one.
func TestEdge156_DiffersFromChrome156ByTrustAnchorsOnly(t *testing.T) {
	edge := wireJA4R(captureWire(t, WithProfile(ProfileEdge156)))
	chrome := wireJA4R(captureWire(t, WithProfile(ProfileChrome156)))
	if !slices.Contains(chrome.exts, trustAnchorsExtensionID) || slices.Contains(edge.exts, trustAnchorsExtensionID) {
		t.Fatalf("trust_anchors: chrome_156 has it = %v, edge_156 has it = %v; want true, false",
			slices.Contains(chrome.exts, trustAnchorsExtensionID), slices.Contains(edge.exts, trustAnchorsExtensionID))
	}
	rest := slices.DeleteFunc(slices.Clone(chrome.exts), func(e uint16) bool { return e == trustAnchorsExtensionID })
	if !slices.Equal(rest, edge.exts) || !slices.Equal(chrome.ciphers, edge.ciphers) || !slices.Equal(chrome.sigalgs, edge.sigalgs) {
		t.Errorf("edge_156 differs from chrome_156 by more than trust_anchors:\n edge   %s\n chrome %s", edge, chrome)
	}

	// The same holds between the real browsers: Chrome 156 and Edge 156 captured
	// on the same Windows runner image with the same harness.
	rc := parseJA4R(t, loadEdgeWire(t, "google_chrome_156_windows_nav.json").TLS.JA4R)
	re := parseJA4R(t, loadEdgeWire(t, "microsoft_edge_156_windows_nav.json").TLS.JA4R)
	rcRest := slices.DeleteFunc(slices.Clone(rc.exts), func(e uint16) bool { return e == trustAnchorsExtensionID })
	if !slices.Equal(rcRest, re.exts) || !slices.Equal(rc.ciphers, re.ciphers) || !slices.Equal(rc.sigalgs, re.sigalgs) {
		t.Errorf("the captures differ by more than trust_anchors:\n edge   %s\n chrome %s", re, rc)
	}
}

// The HTTP/2 preface of the capture (SETTINGS and WINDOW_UPDATE of the Akamai
// string, and the HEADERS priority weight) equals what edge_156 sends.
func TestEdge156_HTTP2EqualsCapture(t *testing.T) {
	for _, tc := range []struct {
		name  string
		file  string
		fetch bool
	}{
		{"navigation", "microsoft_edge_156_windows_nav.json", false},
		{"fetch", "microsoft_edge_156_windows_fetch.json", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ref := loadEdgeWire(t, tc.file)
			opts := []ClientOption{WithProfile(ProfileEdge156)}
			if tc.fetch {
				opts = append(opts, WithFetchMode())
			}
			w := captureWire(t, opts...)
			var settings []string
			for _, s := range w.Settings {
				settings = append(settings, fmt.Sprintf("%d:%d", s[0], s[1]))
			}
			got := strings.Join(settings, ";") + "|" + strconv.FormatUint(uint64(w.WindowUpdate), 10)
			if want, _, _ := strings.Cut(ref.HTTP2.Akamai, "|0|"); got != want {
				t.Errorf("SETTINGS|WINDOW_UPDATE = %s, capture %s", got, want)
			}
			if w.Priority == nil || w.Priority.Weight() != ref.HTTP2.Priority.Weight || !w.Priority.Exclusive || w.Priority.DependsOn != 0 {
				t.Errorf("HEADERS priority = %+v, capture weight %d exclusive on stream 0", w.Priority, ref.HTTP2.Priority.Weight)
			}
		})
	}
}

// Chrome shuffles the extension order per connection and so does Edge: JA3
// varied across every Edge 156 capture.
func TestEdge156_ExtensionOrderShuffles(t *testing.T) {
	orders := map[string]bool{}
	for range 12 {
		orders[fmt.Sprint(captureWire(t, WithProfile(ProfileEdge156)).Extensions)] = true
	}
	if len(orders) < 2 {
		t.Fatal("edge_156 sent the same extension order on 12 connections")
	}
}

// Header values and order equal the captures, for each platform and mode.
func TestEdge156_RequestHeadersEqualCaptures(t *testing.T) {
	for _, tc := range []struct {
		file string
		opts HeaderOptions
		// xhr: only values and the set of names are compared. The captured XHR
		// puts user-agent before sec-ch-ua-mobile, which a fetch does not; the
		// order built for FetchCORS is the fetch one.
		xhr bool
	}{
		{"microsoft_edge_156_windows_nav.json", HeaderOptions{}, false},
		{"microsoft_edge_156_windows_fetch.json", HeaderOptions{Mode: FetchCORS}, false},
		{"microsoft_edge_156_windows_xhr.json", HeaderOptions{Mode: FetchCORS}, true},
		{"microsoft_edge_156_linux_nav.json", HeaderOptions{}, false},
		{"microsoft_edge_156_linux_fetch.json", HeaderOptions{Mode: FetchCORS}, false},
	} {
		t.Run(tc.file, func(t *testing.T) {
			c := loadWire(t, tc.file)
			id := capturedIdentity(t, c.HTTP2.Headers["user-agent"])
			if id.TLSProfile != ProfileEdge156 || id.Browser != "edge" {
				t.Fatalf("captured UA resolves to %s/%s, want edge/edge_156", id.Browser, id.TLSProfile)
			}
			got, err := id.RequestHeaders(tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.xhr {
				assertMatchesCapture(t, got, c)
			} else {
				for name, want := range c.HTTP2.Headers {
					if g := got.Values[name]; g != want {
						t.Errorf("%s = %q, capture has %q", name, g, want)
					}
				}
				a, b := slices.Sorted(slices.Values(got.Order)), slices.Sorted(slices.Values(builtNames(c.HTTP2.Names)))
				if !slices.Equal(a, b) {
					t.Errorf("names\n got  %v\n want %v", a, b)
				}
			}
			if got.Values["priority"] != c.HTTP2.Priority {
				t.Errorf("priority = %q, capture %q", got.Values["priority"], c.HTTP2.Priority)
			}
		})
	}
}

// The client built on edge_156 presents the Windows Edge identity and sends the
// captured order.
func TestEdge156_ClientIdentity(t *testing.T) {
	c := loadWire(t, "microsoft_edge_156_windows_nav.json")
	bc, err := NewClient(WithProfile(ProfileEdge156), WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bc.Close() })
	id := bc.Identity()
	if id.UserAgent != c.HTTP2.Headers["user-agent"] || id.Browser != "edge" || id.OS != "windows" {
		t.Fatalf("identity = %s / %s / %s, capture UA %s", id.Browser, id.OS, id.UserAgent, c.HTTP2.Headers["user-agent"])
	}
	if got := id.ClientHints["sec-ch-ua"]; got != c.HTTP2.Headers["sec-ch-ua"] {
		t.Errorf("sec-ch-ua = %s, capture %s", got, c.HTTP2.Headers["sec-ch-ua"])
	}
	if !slices.Equal(bc.headerOrder, defaultHeaderOrderFor(id, false)) || !slices.Equal(bc.headerOrder[:3], []string{"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform"}) {
		t.Errorf("header order = %v", bc.headerOrder)
	}
}

// The entries and the platform hint: Windows first (profileForTLS returns the
// first match), and each OS's sec-ch-ua-platform is the value Edge sent there.
func TestEdge156_BuiltinEntries(t *testing.T) {
	platformOf := map[string]string{
		"windows": loadWire(t, "microsoft_edge_156_windows_nav.json").HTTP2.Headers["sec-ch-ua-platform"],
		"linux":   loadWire(t, "microsoft_edge_156_linux_nav.json").HTTP2.Headers["sec-ch-ua-platform"],
	}
	seen := map[string]bool{}
	for _, bp := range BuiltinProfiles {
		if bp.TLSProfile != ProfileEdge156 {
			continue
		}
		seen[bp.OS] = true
		want, measured := platformOf[bp.OS]
		if !measured {
			t.Errorf("edge_156 entry for %s, which was not measured", bp.OS)
			continue
		}
		if got := ClientHintsHeaders(bp.UserAgent)["sec-ch-ua-platform"]; got != want {
			t.Errorf("%s: sec-ch-ua-platform = %s, Edge sent %s", bp.OS, got, want)
		}
	}
	if len(seen) != len(platformOf) {
		t.Errorf("edge_156 entries for %v, want one per measured OS %v", seen, platformOf)
	}
	if first, ok := profileForTLS(ProfileEdge156); !ok || first.OS != "windows" || first.Browser != "edge" {
		t.Errorf("profileForTLS(edge_156) = %+v, want the Windows Edge entry", first)
	}
	if got := UserAgentForProfile(ProfileEdge156); !strings.Contains(got, "Windows NT 10.0") || !strings.Contains(got, "Edg/156.") {
		t.Errorf("UserAgentForProfile(edge_156) = %q", got)
	}
}

// A base profile with no trust_anchors extension must fail loudly: the removal
// would otherwise be a silent no-op presenting Chrome's ClientHello as Edge's.
func TestWithoutTrustAnchors_FailsOnBaseWithoutExtension(t *testing.T) {
	p := withoutTrustAnchors(profiles.Chrome_131)
	if _, err := p.GetClientHelloId().SpecFactory(); err == nil {
		t.Fatal("no error for a base profile without trust_anchors")
	}
	if _, err := mapTLSProfile(ProfileEdge156); err != nil {
		t.Fatal(err)
	}
	spec, err := profileMap[ProfileEdge156].GetClientHelloId().SpecFactory()
	if err != nil {
		t.Fatalf("edge_156 ClientHello: %v", err)
	}
	if len(spec.Extensions) == 0 {
		t.Fatal("edge_156 ClientHello has no extensions")
	}
}
