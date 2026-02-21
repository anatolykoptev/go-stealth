package stealth

import "testing"

func TestClientHintsHeaders_Chrome(t *testing.T) {
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	h := ClientHintsHeaders(ua)
	if h == nil {
		t.Fatal("expected hints for Chrome UA")
	}
	if h["sec-ch-ua-platform"] != `"Windows"` {
		t.Fatalf("unexpected platform: %s", h["sec-ch-ua-platform"])
	}
	if h["sec-ch-ua-mobile"] != "?0" {
		t.Fatalf("unexpected mobile: %s", h["sec-ch-ua-mobile"])
	}
}

func TestClientHintsHeaders_Safari(t *testing.T) {
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15"
	h := ClientHintsHeaders(ua)
	if h != nil {
		t.Fatal("expected nil hints for Safari UA")
	}
}

func TestExtractChromeVersion(t *testing.T) {
	tests := []struct {
		ua      string
		version string
	}{
		{"...Chrome/131.0.0.0...", "131"},
		{"...Chrome/130.0...", "130"},
		{"Safari/605.1.15", "131"}, // fallback
	}
	for _, tt := range tests {
		v := ExtractChromeVersion(tt.ua)
		if v != tt.version {
			t.Fatalf("ExtractChromeVersion(%q) = %q, want %q", tt.ua, v, tt.version)
		}
	}
}
