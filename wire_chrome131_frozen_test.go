package stealth

import (
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
	if w.WindowUpdate != 15663105 {
		t.Errorf("WINDOW_UPDATE = %d, want 15663105", w.WindowUpdate)
	}
	if w.Priority == nil || *w.Priority != (h2Priority{Exclusive: true, DependsOn: 0, Wire: 255}) {
		t.Errorf("HEADERS priority = %+v, want exclusive dep 0 weight 256", w.Priority)
	}
}
