package stealth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The oracle for these tests is a real capture, not this package: the Google
// Chrome 156 wire files and the Chrome for Testing references under testdata/.
// An expected value is read from the file, never retyped here.

type browserWireCapture struct {
	HTTP2 struct {
		Names    []string          `json:"request_header_names"`
		Headers  map[string]string `json:"request_headers"`
		Priority string            `json:"priority_header"`
	} `json:"http2"`
}

func loadWire(t *testing.T, name string) browserWireCapture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "wire", name))
	if err != nil {
		t.Fatal(err)
	}
	var c browserWireCapture
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// capturedIdentity returns the BuiltinProfiles identity whose User-Agent is
// the one the capture was taken with (Google Chrome 156 on macOS).
func capturedIdentity(t *testing.T, ua string) BrowserIdentity {
	t.Helper()
	for _, bp := range BuiltinProfiles {
		if bp.UserAgent == ua {
			return BrowserIdentity{BrowserProfile: bp, ClientHints: ClientHintsHeaders(bp.UserAgent)}
		}
	}
	t.Fatalf("no BuiltinProfiles entry has the captured User-Agent %q", ua)
	return BrowserIdentity{}
}

// builtNames keeps the names of a capture this builder is responsible for:
// everything except the page's own headers, referer and cookie (which depend
// on the page, not on the browser) and duplicates.
func builtNames(names []string) []string {
	var out []string
	for _, n := range names {
		switch {
		case n == "authorization", n == "x-csrf-token", n == "content-type",
			n == "referer", n == "cookie", strings.HasPrefix(n, "x-custom-"):
			continue
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

func assertMatchesCapture(t *testing.T, got RequestHeaders, c browserWireCapture) {
	t.Helper()
	if len(got.Values) != len(c.HTTP2.Headers) {
		t.Errorf("got %d headers, capture has %d: %v", len(got.Values), len(c.HTTP2.Headers), got.Values)
	}
	for name, want := range c.HTTP2.Headers {
		if g, ok := got.Values[name]; !ok {
			t.Errorf("missing header %q (capture: %q)", name, want)
		} else if g != want {
			t.Errorf("%s = %q, capture has %q", name, g, want)
		}
	}
	if want := builtNames(c.HTTP2.Names); !slices.Equal(got.Order, want) {
		t.Errorf("order\n got  %v\n want %v", got.Order, want)
	}
}

func TestRequestHeaders_Chrome156Navigation_EqualsCapture(t *testing.T) {
	c := loadWire(t, "google_chrome_156_nav.json")
	id := capturedIdentity(t, c.HTTP2.Headers["user-agent"])
	got, err := id.RequestHeaders(HeaderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesCapture(t, got, c)
	if got.Values["priority"] != c.HTTP2.Priority {
		t.Errorf("priority = %q, capture priority_header %q", got.Values["priority"], c.HTTP2.Priority)
	}
}

func TestRequestHeaders_Chrome156Fetch_EqualsCapture(t *testing.T) {
	c := loadWire(t, "google_chrome_156_fetch.json")
	id := capturedIdentity(t, c.HTTP2.Headers["user-agent"])
	got, err := id.RequestHeaders(HeaderOptions{Mode: FetchCORS})
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesCapture(t, got, c)
	if got.Values["priority"] != c.HTTP2.Priority {
		t.Errorf("priority = %q, capture priority_header %q", got.Values["priority"], c.HTTP2.Priority)
	}
}

// The client method reports the same set for the identity the client presents.
func TestBrowserClient_RequestHeaders_FollowsIdentity(t *testing.T) {
	c := loadWire(t, "google_chrome_156_nav.json")
	id := capturedIdentity(t, c.HTTP2.Headers["user-agent"])
	bc, err := NewClient(WithIdentity(id), WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bc.Close() })
	got, err := bc.RequestHeaders(HeaderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertMatchesCapture(t, got, c)
}

type referenceCapture struct {
	HeaderOrder    []string `json:"header_order"`
	Accept         string   `json:"accept"`
	AcceptLanguage string   `json:"accept_language"`
	AcceptEncoding string   `json:"accept_encoding"`
}

// Chrome for Testing references 131, 133, 144 and 146: accept, accept-language
// and accept-encoding equal the capture, and the navigation names are the same
// set. Order equals the capture for the majors that keep the Chrome for Testing
// order (131, 133, 144); 146 deliberately uses the branded Chrome order
// (ChromeNavigationHeaderOrder), so only its names are compared.
func TestRequestHeaders_ReferenceMajors_EqualCapture(t *testing.T) {
	for _, tc := range []struct {
		tls        TLSProfile
		file       string
		sameOrder  bool
		wantAccept bool
	}{
		{ProfileChrome131, "reference_chrome_131.json", true, true},
		{ProfileChrome133, "reference_chrome_133.json", true, true},
		{ProfileChrome144, "reference_chrome_144.json", true, true},
		{ProfileChrome146, "reference_chrome_146.json", false, true},
	} {
		t.Run(string(tc.tls), func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			var ref referenceCapture
			if err := json.Unmarshal(b, &ref); err != nil {
				t.Fatal(err)
			}
			bp, ok := profileForTLS(tc.tls)
			if !ok {
				t.Fatalf("no profile for %s", tc.tls)
			}
			id := BrowserIdentity{BrowserProfile: bp, ClientHints: ClientHintsHeaders(bp.UserAgent)}
			got, err := id.RequestHeaders(HeaderOptions{})
			if err != nil {
				t.Fatal(err)
			}
			for name, want := range map[string]string{"accept": ref.Accept, "accept-language": ref.AcceptLanguage, "accept-encoding": ref.AcceptEncoding} {
				if got.Values[name] != want {
					t.Errorf("%s = %q, reference %q", name, got.Values[name], want)
				}
			}
			if tc.sameOrder {
				if !slices.Equal(got.Order, ref.HeaderOrder) {
					t.Errorf("order\n got  %v\n want %v", got.Order, ref.HeaderOrder)
				}
				return
			}
			gotSet, wantSet := slices.Sorted(slices.Values(got.Order)), slices.Sorted(slices.Values(ref.HeaderOrder))
			if !slices.Equal(gotSet, wantSet) {
				t.Errorf("names\n got  %v\n want %v", gotSet, wantSet)
			}
		})
	}
}

func TestRequestHeaders_Options(t *testing.T) {
	c := loadWire(t, "google_chrome_156_nav.json")
	id := capturedIdentity(t, c.HTTP2.Headers["user-agent"])

	got, err := id.RequestHeaders(HeaderOptions{Mode: FetchCORS, Site: FetchSiteCrossSite, AcceptLanguage: "ru-RU,ru;q=0.9", OmitAcceptEncoding: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["sec-fetch-site"] != "cross-site" || got.Values["accept-language"] != "ru-RU,ru;q=0.9" {
		t.Errorf("options not applied: %v", got.Values)
	}
	if _, ok := got.Values["accept-encoding"]; ok || slices.Contains(got.Order, "accept-encoding") {
		t.Errorf("accept-encoding present despite OmitAcceptEncoding: %v / %v", got.Values, got.Order)
	}
	if len(got.Order) != len(got.Values) {
		t.Errorf("Order %v does not cover Values %v", got.Order, got.Values)
	}

	if _, err := id.RequestHeaders(HeaderOptions{Site: "bogus"}); err == nil {
		t.Error("unknown Site accepted")
	}
	if _, err := id.RequestHeaders(HeaderOptions{Mode: 99}); err == nil {
		t.Error("unknown Mode accepted")
	}
	// The result is a copy: editing it does not leak into the next call.
	got.Values["user-agent"] = "x"
	again, _ := id.RequestHeaders(HeaderOptions{})
	if again.Values["user-agent"] == "x" {
		t.Error("RequestHeaders returned shared state")
	}
}

func TestRequestHeaders_NonChromiumRefused(t *testing.T) {
	for _, bp := range BuiltinProfiles {
		if bp.Browser != "firefox" && bp.Browser != "safari" {
			continue
		}
		id := BrowserIdentity{BrowserProfile: bp}
		if _, err := id.RequestHeaders(HeaderOptions{}); err == nil {
			t.Errorf("%s: no error", bp.TLSProfile)
		}
		if _, err := id.UserAgentData(""); err == nil {
			t.Errorf("%s: UserAgentData no error", bp.TLSProfile)
		}
	}
}
