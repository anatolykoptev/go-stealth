package stealth

import (
	"strconv"
	"strings"
	"testing"
)

const (
	acceptNoJXL = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	acceptJXL   = "text/html,application/xhtml+xml,application/xml;q=0.9,image/jxl,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	// jxlFirstMajor is the first major measured to advertise image/jxl in the
	// navigation Accept (Chrome 156; Chrome for Testing 152 and 131 do not).
	// The exact version it landed in, between 153 and 156, is not pinned.
	jxlFirstMajor = 156
)

// Navigation Accept follows the Chrome major: 156 added image/jxl, earlier
// majors (131 and 152 measured) do not send it.
func TestNavigationAccept_FollowsChromeMajor(t *testing.T) {
	seen := map[bool]int{}
	for _, set := range [][]BrowserProfile{BuiltinProfiles, legacyProfiles} {
		for _, bp := range set {
			if !strings.Contains(bp.UserAgent, "Chrome/") {
				continue
			}
			maj, err := strconv.Atoi(ExtractChromeVersion(bp.UserAgent))
			if err != nil {
				t.Fatal(err)
			}
			want := acceptNoJXL
			if maj >= jxlFirstMajor {
				want = acceptJXL
			}
			seen[maj >= jxlFirstMajor]++
			if got := ChromeHeadersFor(bp)["accept"]; got != want {
				t.Errorf("%s/%s (Chrome %d): accept = %q, want %q", bp.Browser, bp.OS, maj, got, want)
			}
		}
	}
	if seen[true] == 0 || seen[false] == 0 {
		t.Fatalf("test needs profiles on both sides of the jxl cut: %v", seen)
	}
}

func TestNavigationAccept_DefaultClient(t *testing.T) {
	bc, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bc.Close() })
	if got := bc.ChromeHeaders()["accept"]; got != acceptJXL {
		t.Errorf("default client accept = %q", got)
	}
}
