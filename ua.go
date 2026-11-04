package stealth

import "math/rand/v2"

// RandomUserAgents is a pool of browser User-Agents for rotation, for callers
// that do not care which browser they look like (connectivity probes).
// Its Chrome entries describe ONLY the default TLS profile (ProfileChrome152);
// pairing a User-Agent from here with a client built on another profile
// advertises a different browser than the one fingerprinted. Use
// (*BrowserClient).Identity() or ChromeHeadersFor to get a matching one. Firefox entries are kept in line with ProfileFirefox148.
// The Safari 17.2 entry is stale relative to BuiltinProfiles (17.6/18.2) but
// out of scope here — left as-is.
var RandomUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
	"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:148.0) Gecko/20100101 Firefox/148.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:148.0) Gecko/20100101 Firefox/148.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0",
}

// RandomUserAgent returns a random Chrome-like User-Agent.
func RandomUserAgent() string {
	return RandomUserAgents[rand.IntN(len(RandomUserAgents))]
}

// defaultChromeUserAgents returns the desktop Chrome User-Agents of the default
// client profile, derived from BuiltinProfiles so it cannot drift from the
// default TLS profile (a hand-kept list did exactly that when the default
// moved). Mobile and non-Chrome entries are excluded.
func defaultChromeUserAgents() []string {
	def := defaultConfig().profile
	var uas []string
	for _, bp := range BuiltinProfiles {
		if bp.TLSProfile == def && bp.Browser == "chrome" && !bp.Mobile {
			uas = append(uas, bp.UserAgent)
		}
	}
	return uas
}

// ChromeHeaders returns Chrome navigation headers for the DEFAULT client
// profile: the User-Agent is a random desktop-OS variant of the browser that
// NewClient() presents when no WithProfile/WithIdentity is given, so its major
// matches the default TLS fingerprint. It does not describe a client built with
// another profile; for that use ChromeHeadersFor or (*BrowserClient).ChromeHeaders.
//
// The set and the accept value match the real-Chrome references captured per
// major in internal/fingerprint/testdata/reference_chrome_*.json (header_order
// + accept fields). Real Chrome sends 13 request headers on a top-level GET
// navigation; the sec-ch-ua-* trio is added separately by ClientHintsHeaders
// (it is profile-version-dependent), so ChromeHeaders emits the remaining 10.
// The sec-fetch-*, upgrade-insecure-requests, and priority values are the
// canonical Chrome top-level-navigation values; the references record only
// these headers' names and order (in header_order), not their values, and the
// oracle compares only names+order for them.
func ChromeHeaders() map[string]string {
	uas := defaultChromeUserAgents()
	return chromeNavigationHeaders(uas[rand.IntN(len(uas))])
}

// ChromeHeadersFor returns Chrome navigation headers that describe profile p:
// its own User-Agent plus the sec-ch-ua-* Client Hints derived from it, so the
// advertised browser matches the TLS fingerprint p.TLSProfile. Use it with the
// profile the client was built with (BuiltinProfiles, RandomProfile,
// PlatformMatchedProfile). For a non-Chromium profile (Firefox, Safari) it
// returns the same navigation headers with that profile's own User-Agent and no
// Client Hints, since those browsers send none. The result is never nil.
func ChromeHeadersFor(p BrowserProfile) map[string]string {
	return withHints(chromeNavigationHeaders(p.UserAgent), ClientHintsHeaders(p.UserAgent))
}

// ChromeHeaders returns Chrome navigation headers matching the identity this
// client presents (see Identity): the identity's User-Agent and Client Hints
// over the same header set as the package-level ChromeHeaders. Never nil;
// a non-Chromium identity carries no Client Hints.
func (bc *BrowserClient) ChromeHeaders() map[string]string {
	id := bc.Identity()
	return withHints(chromeNavigationHeaders(id.UserAgent), id.ClientHints)
}

func withHints(h, hints map[string]string) map[string]string {
	for k, v := range hints {
		h[k] = v
	}
	return h
}

func chromeNavigationHeaders(ua string) map[string]string {
	return map[string]string{
		"accept-language":           "en-US,en;q=0.9",
		"upgrade-insecure-requests": "1",
		"user-agent":                ua,
		"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"sec-fetch-site":            "none",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-user":            "?1",
		"sec-fetch-dest":            "document",
		"accept-encoding":           "gzip, deflate, br, zstd",
		"priority":                  "u=0, i",
	}
}
