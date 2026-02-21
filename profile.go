package stealth

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"strings"

	"github.com/bogdanfinn/tls-client/profiles"
)

// BrowserProfile pairs a User-Agent string with a matching TLS fingerprint.
type BrowserProfile struct {
	UserAgent  string
	TLSProfile profiles.ClientProfile
}

// BuiltinProfiles provides browser fingerprint diversity across Chrome, Safari, and Firefox.
// Each entry pairs a UA with the correct TLS fingerprint so JA3 matches the UA claim.
var BuiltinProfiles = []BrowserProfile{
	{
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		TLSProfile: profiles.Chrome_131,
	},
	{
		UserAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15",
		TLSProfile: profiles.Safari_16_0,
	},
	{
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
		TLSProfile: profiles.Firefox_132,
	},
	{
		UserAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		TLSProfile: profiles.Chrome_131,
	},
	{
		UserAgent:  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		TLSProfile: profiles.Chrome_131,
	},
	{
		UserAgent:  "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0",
		TLSProfile: profiles.Firefox_132,
	},
	{
		UserAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.6 Safari/605.1.15",
		TLSProfile: profiles.Safari_16_0,
	},
	{
		UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
		TLSProfile: profiles.Chrome_131,
	},
}

// ClientHintsHeaders returns sec-ch-ua-* headers for Chromium-based UAs.
// Returns nil for Safari/Firefox (they don't send Client Hints).
func ClientHintsHeaders(ua string) map[string]string {
	if !strings.Contains(ua, "Chrome/") {
		return nil
	}
	version := ExtractChromeVersion(ua)
	platform := extractPlatform(ua)
	return map[string]string{
		"sec-ch-ua":          fmt.Sprintf(`"Chromium";v="%s", "Not_A Brand";v="24"`, version),
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": fmt.Sprintf(`"%s"`, platform),
	}
}

// ExtractChromeVersion extracts the major Chrome version from a User-Agent string.
// "...Chrome/131.0.0.0..." -> "131"
func ExtractChromeVersion(ua string) string {
	idx := strings.Index(ua, "Chrome/")
	if idx == -1 {
		return "131"
	}
	rest := ua[idx+7:]
	dot := strings.IndexByte(rest, '.')
	if dot == -1 {
		return rest
	}
	return rest[:dot]
}

// PlatformMatchedProfile returns a BrowserProfile whose User-Agent OS matches
// the actual runtime platform (runtime.GOOS). This prevents fingerprint mismatch
// where e.g. a "Windows" UA is sent from a Linux server.
func PlatformMatchedProfile() BrowserProfile {
	os := runtime.GOOS
	var candidates []BrowserProfile
	for _, p := range BuiltinProfiles {
		if matchesOS(p.UserAgent, os) {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		// Fallback to any profile
		return BuiltinProfiles[rand.IntN(len(BuiltinProfiles))]
	}
	return candidates[rand.IntN(len(candidates))]
}

// matchesOS checks if a User-Agent string matches the given GOOS value.
func matchesOS(ua, goos string) bool {
	switch goos {
	case "windows":
		return strings.Contains(ua, "Windows")
	case "darwin":
		return strings.Contains(ua, "Macintosh")
	case "linux":
		return strings.Contains(ua, "Linux") || strings.Contains(ua, "X11")
	default:
		return false
	}
}

// extractPlatform returns the OS platform from a User-Agent for sec-ch-ua-platform.
func extractPlatform(ua string) string {
	switch {
	case strings.Contains(ua, "Windows"):
		return "Windows"
	case strings.Contains(ua, "Macintosh"):
		return "macOS"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	default:
		return "Windows"
	}
}
