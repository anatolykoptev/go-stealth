package stealth

import (
	"errors"
	"strings"
)

// UserAgentBrand is one entry of a brand list: the name and a version.
type UserAgentBrand struct {
	Brand   string
	Version string
}

// UserAgentData is the structured form of an identity's Client Hints, in the
// shape the Chrome DevTools Protocol takes in
// Emulation.setUserAgentOverride's userAgentMetadata and that
// navigator.userAgentData reports. Brands is the low-entropy list (major
// versions, the same list and order as the sec-ch-ua header) and
// FullVersionList the high-entropy one (sec-ch-ua-full-version-list).
//
// PlatformVersion, Architecture, Bitness, Model and Wow64 are not derivable
// from the User-Agent, so they are left zero for the caller, who knows the
// platform it presents.
type UserAgentData struct {
	Brands          []UserAgentBrand
	FullVersionList []UserAgentBrand
	Mobile          bool
	Platform        string

	PlatformVersion string
	Architecture    string
	Bitness         string
	Model           string
	Wow64           bool
}

// ErrNotChromium is returned by the request-header and UserAgentData builders
// for an identity that is not Chromium-based: Firefox and Safari send no
// Client Hints and their request shape is not modelled here.
var ErrNotChromium = errors.New("stealth: identity is not Chromium-based")

// UserAgentData builds the structured Client Hints of the identity. The brand
// list is produced by the same code as the sec-ch-ua header, so the two agree
// by construction, GREASE brand and order included.
//
// fullVersion is the browser's full version (for example "156.0.8078.80").
// A Chrome User-Agent carries only the reduced "156.0.0.0", so the build and
// patch numbers cannot be derived from the identity; pass the real ones when
// the page can observe them. Empty means the reduced form of the identity's
// own major, "N.0.0.0".
func (id BrowserIdentity) UserAgentData(fullVersion string) (UserAgentData, error) {
	if !id.isChromium() {
		return UserAgentData{}, ErrNotChromium
	}
	ua := id.UserAgent
	major := ExtractChromeVersion(ua)
	if fullVersion == "" {
		fullVersion = major + ".0.0.0"
	}
	brand, brandMajor, brandFull := "Google Chrome", major, fullVersion
	if strings.Contains(ua, "Edg/") {
		brand, brandMajor = "Microsoft Edge", extractEdgeVersion(ua)
		brandFull = brandMajor + ".0.0.0"
		if strings.HasPrefix(fullVersion, brandMajor+".") {
			brandFull = fullVersion
		}
	}
	low := brandList(major, brand, major, brandMajor, "")
	full := brandList(major, brand, fullVersion, brandFull, ".0.0.0")
	return UserAgentData{
		Brands:          low[:],
		FullVersionList: full[:],
		Mobile:          strings.Contains(ua, "Mobile"),
		Platform:        extractPlatform(ua),
	}, nil
}

// isChromium reports whether the identity is a Chromium-family browser whose
// request shape this package models.
func (id BrowserIdentity) isChromium() bool {
	switch id.Browser {
	case "chrome", "edge", "brave":
		return true
	case "":
		return strings.Contains(id.UserAgent, "Chrome/")
	}
	return false
}
