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

// The result must be safe in a header whatever the arguments hold: no control
// character, and the contact cannot close its own comment.
func TestHonestUserAgent_NeutralisesInjection(t *testing.T) {
	for _, tc := range []struct{ name, version, contact, want string }{
		{"tool\r\nX-Evil: 1", "1.0", "", "tool--X-Evil--1/1.0"},
		{"tool", "1\n0", "", "tool/1-0"},
		{"tool", "1.0", "a@b.c\r\nX-Evil: 1", "tool/1.0 (+a@b.c--X-Evil: 1)"},
		{"tool", "1.0", "a\x00b", "tool/1.0 (+a-b)"},
		{"tool", "1.0", "x) Mozilla/5.0 (y", "tool/1.0 (+x- Mozilla/5.0 -y)"},
		{"tool", "1.0", `a\b`, "tool/1.0 (+a-b)"},
		{"tool", "1.0", "caf\u00e9", "tool/1.0 (+caf--)"},
		{"tool", "1.0", " \r\n ", "tool/1.0 (+--)"},
		{"tool", "1.0", "   ", "tool/1.0"},
	} {
		got := HonestUserAgent(tc.name, tc.version, tc.contact)
		if got != tc.want {
			t.Errorf("HonestUserAgent(%q,%q,%q) = %q, want %q", tc.name, tc.version, tc.contact, got, tc.want)
		}
		for i := 0; i < len(got); i++ {
			if got[i] < 0x20 || got[i] > 0x7e {
				t.Errorf("%q: control or non-ASCII byte %#x in %q", tc.contact, got[i], got)
			}
		}
		if strings.Count(got, "(") > 1 || strings.Count(got, ")") > 1 || strings.Contains(got, `\`) {
			t.Errorf("comment escapes its parentheses: %q", got)
		}
	}
}
