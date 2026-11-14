package stealth

import (
	"strings"
	"testing"
)

func TestRandomChromeUserAgent_AlwaysDefaultChrome(t *testing.T) {
	maj := profileMajor(DefaultProfile())
	for i := 0; i < 200; i++ {
		ua := RandomChromeUserAgent()
		if ExtractChromeVersion(ua) != maj || !strings.Contains(ua, "Chrome/") || strings.Contains(ua, "Firefox") || strings.Contains(ua, "Mobile") {
			t.Fatalf("UA %q is not a desktop Chrome %s", ua, maj)
		}
	}
}

func TestHonestUserAgent(t *testing.T) {
	for _, tc := range []struct{ name, version, contact, want string }{
		{"mytool", "1.2.3", "", "mytool/1.2.3"},
		{"mytool", "", "", "mytool"},
		{"mytool", "1.0", "ops@example.com", "mytool/1.0 (+ops@example.com)"},
		{"my tool/x", "1 0", "", "my-tool-x/1-0"},
		{"", "", "", "go-client"},
	} {
		got := HonestUserAgent(tc.name, tc.version, tc.contact)
		if got != tc.want {
			t.Errorf("HonestUserAgent(%q,%q,%q) = %q, want %q", tc.name, tc.version, tc.contact, got, tc.want)
		}
		if strings.Contains(got, "Mozilla") {
			t.Errorf("honest UA claims a browser: %q", got)
		}
	}
}
