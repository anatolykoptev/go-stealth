package stealth

import (
	"fmt"
	"maps"
	"slices"
)

// FetchMode is the kind of request a header set describes. Chrome sends a
// different set, values and order for a top-level navigation than for a
// fetch() or XMLHttpRequest issued by a page.
type FetchMode int

const (
	// FetchNavigate is a top-level document navigation (the default).
	FetchNavigate FetchMode = iota
	// FetchCORS is a fetch() or XMLHttpRequest issued by a page: accept
	// "*/*", sec-fetch-mode cors, sec-fetch-dest empty, priority "u=1, i".
	FetchCORS
)

// FetchSite is the value of sec-fetch-site: how the request's initiator
// relates to its target.
type FetchSite string

const (
	// FetchSiteNone is a navigation the user typed or picked from a bookmark.
	FetchSiteNone FetchSite = "none"
	// FetchSiteSameOrigin is a request to the initiator's own origin.
	FetchSiteSameOrigin FetchSite = "same-origin"
	// FetchSiteSameSite is a request to another origin of the same site.
	FetchSiteSameSite FetchSite = "same-site"
	// FetchSiteCrossSite is a request to a different site.
	FetchSiteCrossSite FetchSite = "cross-site"
)

// defaultAcceptLanguage is the accept-language of the captured Chrome and of
// every header set this package builds when no locale is asked for.
const defaultAcceptLanguage = "en-US,en;q=0.9"

// acceptEncodingChrome is the accept-encoding of Chrome 123 and later.
const acceptEncodingChrome = "gzip, deflate, br, zstd"

// navigationPriorityHeader is the priority of a top-level navigation.
const navigationPriorityHeader = "u=0, i"

// HeaderOptions selects the variant of a request header set. The zero value
// is a top-level navigation with Chrome's default locale and accept-encoding.
type HeaderOptions struct {
	// Mode is FetchNavigate (zero value) or FetchCORS.
	Mode FetchMode
	// Site is the sec-fetch-site value. Empty means FetchSiteNone for a
	// navigation and FetchSiteSameOrigin for FetchCORS.
	Site FetchSite
	// AcceptLanguage overrides the accept-language value, for a caller whose
	// locale is not en-US. Empty keeps "en-US,en;q=0.9".
	AcceptLanguage string
	// OmitAcceptEncoding leaves accept-encoding out, for a caller that
	// decodes nothing itself and wants an identity-encoded body.
	OmitAcceptEncoding bool
}

// RequestHeaders is a browser request header set: lower-case names with their
// values, and the order Chrome writes them in. Pass Values to a request and
// Order to WithHeaderOrder or DoWithHeaderOrder. Order lists exactly the names
// in Values.
type RequestHeaders struct {
	Values map[string]string
	Order  []string
}

// RequestHeaders builds the request headers a Chromium browser with this
// identity sends: user-agent, the sec-ch-ua Client Hints derived from it and
// the navigation or fetch set of opts, with their values and Chrome's order.
//
// Values come from the Google Chrome 156 captures in testdata/wire
// (google_chrome_156_nav.json, google_chrome_156_fetch.json); the accept value
// of earlier majors from testdata/reference_chrome_*.json, and the order is
// the one the client itself uses for this identity (see ChromeNavigationHeaderOrder
// and ChromeFetchHeaderOrder). referer, cookie and a page's own headers are
// not part of the set; add them to Values and keep Order's positions with
// ChromeFetchHeaderOrder(custom) when you need more than the built names.
//
// A Firefox or Safari identity returns ErrNotChromium. An unknown Site is an
// error. The result is a fresh copy the caller may modify.
func (id BrowserIdentity) RequestHeaders(opts HeaderOptions) (RequestHeaders, error) {
	if !id.isChromium() {
		return RequestHeaders{}, ErrNotChromium
	}
	site, err := opts.site()
	if err != nil {
		return RequestHeaders{}, err
	}
	lang := opts.AcceptLanguage
	if lang == "" {
		lang = defaultAcceptLanguage
	}

	h := map[string]string{
		"user-agent":      id.UserAgent,
		"accept-language": lang,
		"sec-fetch-site":  string(site),
	}
	if !opts.OmitAcceptEncoding {
		h["accept-encoding"] = acceptEncodingChrome
	}
	hints := id.ClientHints
	if hints == nil {
		hints = ClientHintsHeaders(id.UserAgent)
	}
	for k, v := range hints {
		h[k] = v
	}
	switch opts.Mode {
	case FetchNavigate:
		h["accept"] = navigationAcceptFor(id.UserAgent)
		h["upgrade-insecure-requests"] = "1"
		h["sec-fetch-mode"] = "navigate"
		h["sec-fetch-user"] = "?1"
		h["sec-fetch-dest"] = "document"
		h["priority"] = navigationPriorityHeader
	case FetchCORS:
		h["accept"] = "*/*"
		h["sec-fetch-mode"] = "cors"
		h["sec-fetch-dest"] = "empty"
		h["priority"] = fetchPriorityHeader
	default:
		return RequestHeaders{}, fmt.Errorf("stealth: unknown FetchMode %d", opts.Mode)
	}

	var order []string
	for _, name := range defaultHeaderOrderFor(id, opts.Mode == FetchCORS) {
		if _, ok := h[name]; ok {
			order = append(order, name)
		}
	}
	// A name the order table does not know goes last, so Order always covers Values.
	for _, name := range slices.Sorted(maps.Keys(h)) {
		if !slices.Contains(order, name) {
			order = append(order, name)
		}
	}
	return RequestHeaders{Values: h, Order: order}, nil
}

func (o HeaderOptions) site() (FetchSite, error) {
	switch o.Site {
	case "":
		if o.Mode == FetchCORS {
			return FetchSiteSameOrigin, nil
		}
		return FetchSiteNone, nil
	case FetchSiteNone, FetchSiteSameOrigin, FetchSiteSameSite, FetchSiteCrossSite:
		return o.Site, nil
	}
	return "", fmt.Errorf("stealth: unknown FetchSite %q", string(o.Site))
}

// RequestHeaders builds the request headers for the identity this client
// presents (see Identity), so the User-Agent, Client Hints and order always
// agree with the TLS profile the client was built with. See
// BrowserIdentity.RequestHeaders.
func (bc *BrowserClient) RequestHeaders(opts HeaderOptions) (RequestHeaders, error) {
	return bc.Identity().RequestHeaders(opts)
}
