package stealth

import "slices"

// ChromeNavigationHeaderOrder is the request-header order of a top-level
// navigation in stable Google Chrome 156, taken from a capture of Google Chrome
// 156.0.8078.80 (testdata/wire/google_chrome_156_nav.json). It is the order
// the default client (chrome_156) uses.
//
// It differs from DefaultHeaderOrder in one place: accept-language comes after
// accept-encoding. DefaultHeaderOrder was captured from Chrome for Testing,
// which puts accept-language right after sec-ch-ua-platform; branded Google
// Chrome does not. The difference follows the build, not the version (Chrome
// for Testing 131, 152 and 156 all have the early placement), so every modern
// Chrome profile whose TLS extension order is shuffled (chrome_146, 150, 152,
// 156) uses this order, and chrome_131, 133 and 144 keep DefaultHeaderOrder.
// Branded 146 to 152 were not captured themselves; the placement is carried
// over from the branded 156 capture on the build argument.
//
// referer and cookie are absent from a first request, so the capture does not
// place them; they take the positions they have in the fetch captures (referer
// before accept-encoding, cookie between accept-language and priority), which
// share the same network stack.
var ChromeNavigationHeaderOrder = []string{
	"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
	"upgrade-insecure-requests", "user-agent", "accept",
	"sec-fetch-site", "sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest",
	"referer", "accept-encoding", "accept-language", "cookie", "priority",
}

// braveNavigationHeaderOrder is the order of Brave 156's navigation
// (testdata capture ref_brave_nav): the branded Chrome order plus sec-gpc right
// after accept. go-stealth does not send sec-gpc itself; a caller that does
// gets it in Brave's position.
var braveNavigationHeaderOrder = []string{
	"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
	"upgrade-insecure-requests", "user-agent", "accept", "sec-gpc",
	"sec-fetch-site", "sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest",
	"referer", "accept-encoding", "accept-language", "cookie", "priority",
}

// chromeFetchTail is the part of the fetch order after the page's headers.
var chromeFetchTail = []string{
	"sec-ch-ua", "sec-ch-ua-mobile", "user-agent", "accept",
	"sec-fetch-site", "sec-fetch-mode", "sec-fetch-dest", "referer",
	"accept-encoding", "accept-language", "cookie", "priority",
}

// ChromeFetchHeaderOrder returns the request-header order of a fetch() or
// XMLHttpRequest in stable Google Chrome 156, for use with WithHeaderOrder or
// DoWithHeaderOrder. custom lists the page's own headers (authorization,
// x-csrf-token, ...) in the order you send them; they are placed as one block
// after sec-ch-ua-platform, before the rest.
//
// Headers missing from an order are sent after the listed ones in
// alphabetical order (fhttp), so a custom header you leave out ends up after
// priority. A client built with WithFetchMode and no WithHeaderOrder uses
// ChromeFetchHeaderOrder(nil).
//
// What is established and what is not. Established, from the captures
// testdata/wire/google_chrome_156_fetch.json and the Chrome for Testing
// fetches: sec-ch-ua-platform first, user-agent after the page's headers,
// then accept, sec-fetch-site, sec-fetch-mode, sec-fetch-dest, referer,
// accept-encoding, accept-language (branded Chrome only), cookie, priority.
// Not established: where Chrome interleaves the page's own headers with
// sec-ch-ua, sec-ch-ua-mobile and user-agent. In the capture they are spread
// between them in a pattern that one sample cannot generalise (and sec-ch-ua-mobile
// and user-agent swap places when the page sets no headers); this function
// puts the page's headers in one block. A caller that has captured its own
// target's exact order should pass that order itself.
func ChromeFetchHeaderOrder(custom []string) []string {
	out := make([]string, 0, 1+len(custom)+len(chromeFetchTail))
	out = append(out, "sec-ch-ua-platform")
	out = append(out, custom...)
	return append(out, chromeFetchTail...)
}

// defaultHeaderOrderFor picks the header order a client uses when the caller
// gave none: fetch mode gets the fetch order, the modern Chrome profiles the
// branded-Chrome navigation order, Brave its own, every other profile (chrome_131,
// 133, 144, Edge, Firefox, Safari) the order it has always had.
func defaultHeaderOrderFor(id BrowserIdentity, fetchMode bool) []string {
	switch {
	case fetchMode:
		return ChromeFetchHeaderOrder(nil)
	case id.Browser == "chrome" && extensionOrderRandomised[id.TLSProfile]:
		return slices.Clone(ChromeNavigationHeaderOrder)
	case id.Browser == "brave" && id.TLSProfile == ProfileBrave146:
		return slices.Clone(braveNavigationHeaderOrder)
	default:
		return DefaultHeaderOrderFor(id.Browser)
	}
}
