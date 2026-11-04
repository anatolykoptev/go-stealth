package stealth

import (
	"fmt"
	"slices"
	"testing"
)

// chrome_131 is presented on purpose by existing consumers: it is tls-client's
// contributed Chrome 131 profile, which differs from Chrome-for-Testing 131 (it
// carries the old ALPS codepoint 0x4469 where Chrome 131 sends 0x44cd, and no
// GREASE HTTP/2 SETTINGS entry). This file pins its wire byte for byte, so any
// change to what chrome_131 sends — a tls-client bump, a client-wide option
// meant for newer profiles — fails here instead of silently moving consumers.
//
// Golden values were captured from this code at v1.26.1 (before the
// wire-fidelity changes) through the local wire oracle in wire_capture_test.go.
func TestWire_Chrome131Frozen(t *testing.T) {
	wantCiphers := []uint16{0x0a0a, 0x1301, 0x1302, 0x1303, 0xc02b, 0xc02f, 0xc02c, 0xc030, 0xcca9, 0xcca8, 0xc013, 0xc014, 0x9c, 0x9d, 0x2f, 0x35}
	// ALPS is the old codepoint 0x4469, not 0x44cd.
	wantExts := []uint16{0x0a0a, 0xd, 0xfe0d, 0xff01, 0x12, 0x1b, 0x10, 0x5, 0xa, 0x4469, 0xb, 0x33, 0x23, 0x2b, 0x2d, 0x0, 0x17, 0x0a0a}
	wantSettings := [][2]uint32{{1, 65536}, {2, 0}, {4, 6291456}, {6, 262144}}

	w := captureWire(t, WithProfile(ProfileChrome131))
	if !sameU16(w.Ciphers, wantCiphers) {
		t.Errorf("cipher suites moved:\n got %#x\nwant %#x", w.Ciphers, wantCiphers)
	}
	if !sameU16(w.Extensions, wantExts) {
		t.Errorf("extension order moved:\n got %#x\nwant %#x", w.Extensions, wantExts)
	}
	if !slices.Equal(w.Settings, wantSettings) {
		t.Errorf("HTTP/2 SETTINGS moved: got %v want %v", w.Settings, wantSettings)
	}
	// Deterministic extension payloads, GREASE normalised: signature
	// algorithms, groups, ALPN, ALPS (old codepoint), versions, certificate
	// compression, PSK modes, point formats and the empty-bodied extensions.
	wantPayloads := map[uint16]string{
		0x0d:   "001004030804040105030805050108060601",
		0x0a:   "000a0a0a11ec001d00170018",
		0x10:   "000c02683208687474702f312e31",
		0x4469: "0003026832",
		0x2b:   "060a0a03040303",
		0x1b:   "020002",
		0x2d:   "0101",
		0x0b:   "0100",
		0x05:   "0100000000",
		0xff01: "00",
		0x12:   "",
		0x17:   "",
		0x23:   "",
	}
	for id, want := range wantPayloads {
		if got := payloadHex(w, id); got != want {
			t.Errorf("extension %#x payload moved: got %q want %q", id, got, want)
		}
	}
	if w.WindowUpdate != 15663105 {
		t.Errorf("WINDOW_UPDATE = %d, want 15663105", w.WindowUpdate)
	}
	if w.Priority == nil || *w.Priority != (h2Priority{Exclusive: true, DependsOn: 0, Wire: 255}) {
		t.Errorf("HEADERS priority = %+v, want exclusive dep 0 weight 256", w.Priority)
	}
}

// The headers a chrome_131 client sends by default are part of the frozen
// wire too: the navigation set and the header order.
func TestChrome131Frozen_DefaultHeaders(t *testing.T) {
	bp, ok := profileForTLS(ProfileChrome131)
	if !ok {
		t.Fatal("chrome_131 not resolvable")
	}
	want := map[string]string{
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"accept-encoding":           "gzip, deflate, br, zstd",
		"accept-language":           "en-US,en;q=0.9",
		"priority":                  "u=0, i",
		"sec-ch-ua":                 `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		"sec-ch-ua-mobile":          "?0",
		"sec-ch-ua-platform":        `"Windows"`,
		"sec-fetch-dest":            "document",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-site":            "none",
		"sec-fetch-user":            "?1",
		"upgrade-insecure-requests": "1",
		"user-agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	}
	got := ChromeHeadersFor(bp)
	if len(got) != len(want) {
		t.Errorf("header set changed: got %d headers, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("header %s = %q, want %q", k, got[k], v)
		}
	}

	wantOrder := []string{
		"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "accept-language", "upgrade-insecure-requests",
		"user-agent", "accept", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest",
		"accept-encoding", "priority", "referer", "cookie",
	}
	if order := orderOf(t, WithProfile(ProfileChrome131)); !slices.Equal(order, wantOrder) {
		t.Errorf("chrome_131 client header order moved:\n got  %v\n want %v", order, wantOrder)
	}
	if !slices.Equal(DefaultHeaderOrder, wantOrder) {
		t.Errorf("DefaultHeaderOrder moved: %v", DefaultHeaderOrder)
	}
}

// Consumers build chrome_131 through WithIdentity (a stored Windows Chrome/131
// identity), not WithProfile. That path must be the same frozen wire: same
// ClientHello in a constant order, same SETTINGS and priority, same header
// order, and the Client Hints derived from the User-Agent.
func TestChrome131Frozen_ViaWithIdentity(t *testing.T) {
	id := BrowserIdentity{BrowserProfile: BrowserProfile{
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		TLSProfile: ProfileChrome131, Browser: "chrome", OS: "windows",
	}}

	wantExts := []uint16{0x0a0a, 0xd, 0xfe0d, 0xff01, 0x12, 0x1b, 0x10, 0x5, 0xa, 0x4469, 0xb, 0x33, 0x23, 0x2b, 0x2d, 0x0, 0x17, 0x0a0a}
	orders := map[string]bool{}
	var w wireCapture
	for range extOrderSamples {
		w = captureWire(t, WithIdentity(id))
		orders[fmt.Sprint(w.Extensions)] = true
	}
	if len(orders) != 1 {
		t.Errorf("%d distinct extension orders via WithIdentity, want 1", len(orders))
	}
	if !sameU16(w.Extensions, wantExts) {
		t.Errorf("extension order moved:\n got %#x\nwant %#x", w.Extensions, wantExts)
	}
	wantSettings := [][2]uint32{{1, 65536}, {2, 0}, {4, 6291456}, {6, 262144}}
	if !slices.Equal(w.Settings, wantSettings) || w.WindowUpdate != 15663105 {
		t.Errorf("HTTP/2 preface moved: %v / %d", w.Settings, w.WindowUpdate)
	}
	if w.Priority == nil || *w.Priority != (h2Priority{Exclusive: true, Wire: 255}) {
		t.Errorf("HEADERS priority = %+v", w.Priority)
	}

	wantOrder := []string{
		"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "accept-language", "upgrade-insecure-requests",
		"user-agent", "accept", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest",
		"accept-encoding", "priority", "referer", "cookie",
	}
	if got := orderOf(t, WithIdentity(id)); !slices.Equal(got, wantOrder) {
		t.Errorf("header order via WithIdentity moved:\n got  %v\n want %v", got, wantOrder)
	}

	bc, err := NewClient(WithIdentity(id), WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bc.Close() })
	wantHints := map[string]string{
		"sec-ch-ua":          `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
	}
	got := bc.Identity().ClientHints
	if len(got) != len(wantHints) {
		t.Errorf("client hints = %v", got)
	}
	for k, v := range wantHints {
		if got[k] != v {
			t.Errorf("hint %s = %q, want %q", k, got[k], v)
		}
	}
	if bc.Identity().TLSProfile != ProfileChrome131 {
		t.Errorf("identity profile = %s", bc.Identity().TLSProfile)
	}
}
