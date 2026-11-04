package stealth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func zstdEncode(t *testing.T, plain []byte) []byte {
	t.Helper()
	enc, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = enc.Close() }()
	return enc.EncodeAll(plain, nil)
}

// Chrome has advertised zstd since 123, so servers answer with it.
func TestDecompressBody_Zstd(t *testing.T) {
	plain := []byte(strings.Repeat("zstd body ", 200))
	packed := zstdEncode(t, plain)

	for _, enc := range []string{"zstd", "ZSTD", " zstd "} {
		got, err := decompressBody(packed, enc)
		if err != nil || !bytes.Equal(got, plain) {
			t.Errorf("content-encoding %q: got %d bytes, err=%v; want the %d-byte original", enc, len(got), err, len(plain))
		}
	}
	// Header stripped by an intermediary: the zstd frame magic is enough.
	if got, _ := decompressBody(packed, ""); !bytes.Equal(got, plain) {
		t.Errorf("no content-encoding: zstd frame not recognised by its magic bytes (%d bytes)", len(got))
	}
}

// A zstd stream may open with a skippable frame (RFC 8878 section 3.1.2), so
// its first bytes are not the usual frame magic. Only the content-encoding
// header identifies such a body; this keeps that path from being redundant
// with the magic-byte fallback.
func TestDecompressBody_ZstdSkippableFrameFirst(t *testing.T) {
	plain := []byte(strings.Repeat("after a skippable frame ", 50))
	skippable := []byte{0x50, 0x2a, 0x4d, 0x18, 3, 0, 0, 0, 'x', 'y', 'z'}
	stream := append(skippable, zstdEncode(t, plain)...)

	got, err := decompressBody(stream, "zstd")
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("with content-encoding zstd: got %d bytes, err=%v; want the %d-byte original", len(got), err, len(plain))
	}
}

// Invalid input is returned as received, like the gzip and deflate paths.
func TestDecompressBody_ZstdInvalidReturnsRaw(t *testing.T) {
	junk := []byte("definitely not zstd")
	got, err := decompressBody(junk, "zstd")
	if err != nil || !bytes.Equal(got, junk) {
		t.Fatalf("got %q err=%v, want the raw bytes back", got, err)
	}
}

// Both built-in backends must hand the caller the decoded body of a zstd
// response, with Chrome's accept-encoding on the request.
func TestBackends_DecodeZstdResponse(t *testing.T) {
	plain := []byte(strings.Repeat("zstd over the wire ", 100))
	packed := zstdEncode(t, plain)
	var sawAE string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAE = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Encoding", "zstd")
		_, _ = w.Write(packed)
	}))
	defer srv.Close()

	for name, opt := range map[string]ClientOption{"tls-client": WithProfile(ProfileChrome152), "std": WithStdHTTP()} {
		t.Run(name, func(t *testing.T) {
			bc, err := NewClient(opt, WithoutSSRFGuard())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = bc.Close() })
			hdr := map[string]string{"accept-encoding": "gzip, deflate, br, zstd"}
			body, _, status, err := bc.Do("GET", srv.URL, hdr, nil)
			if err != nil || status != 200 {
				t.Fatalf("status=%d err=%v", status, err)
			}
			if !bytes.Equal(body, plain) {
				t.Fatalf("body not decoded: %d bytes (starts % x), want %d plain bytes", len(body), body[:min(8, len(body))], len(plain))
			}
			if !strings.Contains(sawAE, "zstd") {
				t.Fatalf("server saw accept-encoding %q, want zstd advertised", sawAE)
			}
		})
	}
}

// Every Chrome-family profile's default request headers advertise zstd, as
// Chrome does since 123 (chrome_131 included: its default already does, and
// this pins it so it stays that way).
func TestChromeHeaders_AdvertiseZstd(t *testing.T) {
	seen := 0
	for _, set := range [][]BrowserProfile{BuiltinProfiles, legacyProfiles} {
		for _, bp := range set {
			if bp.Browser != "chrome" && bp.Browser != "edge" && bp.Browser != "brave" {
				continue
			}
			seen++
			if ae := ChromeHeadersFor(bp)["accept-encoding"]; ae != "gzip, deflate, br, zstd" {
				t.Errorf("%s/%s/%s: accept-encoding = %q, want %q", bp.Browser, bp.TLSProfile, bp.OS, ae, "gzip, deflate, br, zstd")
			}
		}
	}
	if seen == 0 {
		t.Fatal("no Chromium profiles seen")
	}
}

// A few kilobytes of zstd can describe hundreds of megabytes. The decoder caps
// one decoded body at 64 MiB, far above any page or API response, so a hostile
// response cannot make the client allocate half a gigabyte. Over the cap the
// body is returned as received (the same tolerant path as invalid input).
func TestDecompressBody_ZstdBombIsBounded(t *testing.T) {
	const decoded = 65 << 20 // just over the 64 MiB cap
	packed := zstdEncode(t, make([]byte, decoded))
	if len(packed) > 1<<20 {
		t.Fatalf("test precondition: %d bytes of zeros packed to %d bytes", decoded, len(packed))
	}
	got, err := decompressBody(packed, "zstd")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) >= decoded {
		t.Fatalf("decoded %d bytes from a %d byte body, want it refused past the cap", len(got), len(packed))
	}

	// Under the cap still decodes.
	small := zstdEncode(t, make([]byte, 8<<20))
	if got, _ := decompressBody(small, "zstd"); len(got) != 8<<20 {
		t.Fatalf("8 MiB body decoded to %d bytes", len(got))
	}
}
