package stealth

import (
	"math/rand/v2"
	"strconv"
	"strings"
)

// RandomUserAgents is a pool of browser User-Agents for rotation, for callers
// that do not care which browser they look like (connectivity probes).
// Its Chrome entries describe ONLY the default TLS profile (ProfileChrome156);
// pairing a User-Agent from here with a client built on another profile
// advertises a different browser than the one fingerprinted. Use
// (*BrowserClient).Identity() or ChromeHeadersFor to get a matching one. Firefox entries are kept in line with ProfileFirefox148.
// The Safari 17.2 entry is stale relative to BuiltinProfiles (17.6/18.2) but
// out of scope here — left as-is.
//
// Deprecated: the pool mixes Chrome, Firefox and Safari, so a User-Agent drawn
// from it over a Chrome TLS fingerprint (any BrowserClient built on a
// chrome_* profile) names a different browser than the handshake shows, and
// drawn per request it changes browser family mid-session. For a client use
// (*BrowserClient).Identity().UserAgent or (*BrowserClient).RequestHeaders; for a
// plain net/http request use DefaultIdentity().UserAgent,
// RandomChromeUserAgent, or HonestUserAgent when no browser identity is
// needed.
var RandomUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
	"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:148.0) Gecko/20100101 Firefox/148.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:148.0) Gecko/20100101 Firefox/148.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0",
}

// RandomUserAgent returns a random entry of RandomUserAgents: Chrome, Firefox
// or Safari, whatever the pool holds, not only Chrome.
//
// Deprecated: it is not safe to pair with a Chrome TLS fingerprint, because
// roughly half of the pool is not Chrome (see RandomUserAgents). Use
// RandomChromeUserAgent for a Chrome User-Agent that matches the default
// profile, (*BrowserClient).Identity().UserAgent for the one a client presents,
// or HonestUserAgent for an API that needs no browser identity.
func RandomUserAgent() string {
	return RandomUserAgents[rand.IntN(len(RandomUserAgents))]
}

// RandomChromeUserAgent returns the User-Agent of a random desktop OS variant
// of the default Chrome profile (see DefaultProfile). It is always Chrome and
// always the major of the default TLS fingerprint, so it is the safe
// replacement for RandomUserAgent on a client built with no WithProfile. For
// a client built with another profile, take the User-Agent from its Identity().
func RandomChromeUserAgent() string {
	return pickChromeUserAgent(defaultChromeUserAgents())
}

// HonestUserAgent returns a User-Agent that names the calling program rather
// than a browser, in the RFC 9110 product form "name/version (+contact)". Use
// it for plain net/http callers whose TLS handshake is Go's own: a browser
// User-Agent over that handshake describes a client that is not there, and
// an API that expects no browser needs none.
//
// It sanitizes instead of returning an error, so the call stays a one-liner
// and its result is always safe to put in a header, even for callers that
// serialize headers themselves: name and version keep only RFC 9110 token
// characters (anything else becomes '-'; an empty name becomes "go-client",
// an empty version is omitted), and contact keeps only printable ASCII with
// '(', ')' and '\' also replaced by '-', so control characters (CR, LF, NUL)
// cannot end a header or close the comment. A contact that is empty after
// trimming is omitted.
func HonestUserAgent(name, version, contact string) string {
	ua := productToken(name, "go-client")
	if v := productToken(version, ""); v != "" {
		ua += "/" + v
	}
	if c := commentText(contact); c != "" {
		ua += " (+" + c + ")"
	}
	return ua
}

// commentText makes s safe inside an RFC 9110 comment: printable ASCII only,
// and no parenthesis or backslash.
func commentText(s string) string {
	out := []byte(s)
	for i := 0; i < len(out); i++ {
		if c := out[i]; c < 0x20 || c > 0x7e || c == '(' || c == ')' || c == '\\' {
			out[i] = '-'
		}
	}
	return strings.TrimSpace(string(out))
}

// productToken keeps the RFC 9110 tchar set and maps anything else to '-'.
func productToken(s, fallback string) string {
	out := []byte(s)
	for i, c := range out {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c != 0 && strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			out[i] = '-'
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return string(out)
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
//
// Deprecated: the result describes the default profile, not the client that
// sends it. A client built with WithProfile or WithIdentity, or a module whose
// go-stealth pin moved the default, gets a User-Agent of one Chrome major over
// the TLS fingerprint of another, and the OS variant changes on every call.
// It also carries no sec-ch-ua-* hints. Use (*BrowserClient).ChromeHeaders or
// (*BrowserClient).RequestHeaders, which follow the client's identity.
func ChromeHeaders() map[string]string {
	return chromeNavigationHeaders(pickChromeUserAgent(defaultChromeUserAgents()))
}

// pickChromeUserAgent returns a random entry of uas, or the default profile's
// own User-Agent when uas is empty, so ChromeHeaders can never panic.
func pickChromeUserAgent(uas []string) string {
	if len(uas) == 0 {
		return UserAgentForProfile(defaultConfig().profile)
	}
	return uas[rand.IntN(len(uas))]
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

// navigationAcceptFrom156 is the navigation Accept of Chrome 156 and later:
// image/jxl is listed ahead of image/avif. Chrome for Testing 131 and 152 do
// not send it; 156 does (both branded and Chrome for Testing). The version
// between 153 and 156 in which it first appeared is not pinned, so the cut is
// at the first major measured with it.
const (
	navigationAccept        = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	navigationAcceptFrom156 = "text/html,application/xhtml+xml,application/xml;q=0.9,image/jxl,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	jxlAcceptFirstMajor     = 156
)

// navigationAcceptFor returns the navigation Accept for the Chrome major in ua.
// Edge never lists image/jxl in the majors measured (Edge 156 does not, where
// Chrome 156 does; testdata/wire/microsoft_edge_156_*_nav.json).
func navigationAcceptFor(ua string) string {
	if strings.Contains(ua, "Edg/") {
		return navigationAccept
	}
	if maj, err := strconv.Atoi(ExtractChromeVersion(ua)); err == nil && maj >= jxlAcceptFirstMajor {
		return navigationAcceptFrom156
	}
	return navigationAccept
}

func chromeNavigationHeaders(ua string) map[string]string {
	return map[string]string{
		"accept-language":           "en-US,en;q=0.9",
		"upgrade-insecure-requests": "1",
		"user-agent":                ua,
		"accept":                    navigationAcceptFor(ua),
		"sec-fetch-site":            "none",
		"sec-fetch-mode":            "navigate",
		"sec-fetch-user":            "?1",
		"sec-fetch-dest":            "document",
		"accept-encoding":           "gzip, deflate, br, zstd",
		"priority":                  "u=0, i",
	}
}
