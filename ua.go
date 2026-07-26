package stealth

import "math/rand/v2"

// RandomUserAgents is a pool of Chrome-like User-Agents for rotation.
// Chrome entries are kept in line with the newest default TLS profile
// (ProfileChrome146). The Safari 17.2 and Firefox 115 entries are stale but
// out of scope for the Chrome-146 profile change — left as-is.
var RandomUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
	"Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/115.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:109.0) Gecko/20100101 Firefox/115.0",
}

// RandomUserAgent returns a random Chrome-like User-Agent.
func RandomUserAgent() string {
	return RandomUserAgents[rand.IntN(len(RandomUserAgents))]
}

// chromeUserAgents is the Chrome-only subset of RandomUserAgents. ChromeHeaders
// advertises Chrome-specific accept/accept-encoding and is paired with a Chrome
// TLS profile, so its User-Agent MUST be a Chrome UA — sourcing it from the
// mixed RandomUserAgents pool (which also holds Safari 17.2 and Firefox 115
// entries) can return a Firefox UA over Chrome headers/JA3, the same
// UA<->JA3 mismatch class this package exists to eliminate. RandomUserAgents
// itself is left untouched: its other caller (proxy.go ValidateProxy) is a
// connectivity probe where browser identity is irrelevant, and the stale
// Safari/Firefox entries are a separate out-of-scope cleanup.
var chromeUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
}

// randomChromeUserAgent returns a random Chrome User-Agent from the Chrome-only
// pool, for use by ChromeHeaders.
func randomChromeUserAgent() string {
	return chromeUserAgents[rand.IntN(len(chromeUserAgents))]
}

// ChromeHeaders returns common Chrome browser headers with a random Chrome User-Agent.
func ChromeHeaders() map[string]string {
	return map[string]string{
		"accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		"accept-language": "en-US,en;q=0.9",
		"accept-encoding": "gzip, deflate, br, zstd",
		"user-agent":      randomChromeUserAgent(),
	}
}
