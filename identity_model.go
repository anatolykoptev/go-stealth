package stealth

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/anatolykoptev/go-stealth/internal/identitymodel"
)

// ErrNoIdentity is returned when the constraints leave no browser identity
// that both the model knows and a built-in TLS profile backs.
var ErrNoIdentity = errors.New("stealth: no browser identity satisfies the constraints")

// Constraints narrows the identities GenerateIdentity and IdentityDistribution
// draw from. The zero value means: every built-in profile, desktop only,
// every OS, locale en-US.
type Constraints struct {
	// Profiles lists the TLS profiles an identity may ride. Empty means the
	// profiles of BuiltinProfiles. A profile outside BuiltinProfiles (for
	// example a legacy one) is used only when listed here, so a frozen
	// profile is never drawn by accident.
	Profiles []TLSProfile `json:"profiles,omitempty"`
	// OS limits the operating systems ("windows", "macos", "linux",
	// "android", "ios"). Empty means all.
	OS []string `json:"os,omitempty"`
	// IncludeMobile also draws mobile identities, in the model's
	// desktop/mobile proportion. Off by default: the built-in TLS profiles
	// were measured as desktop Chrome.
	IncludeMobile bool `json:"include_mobile,omitempty"`
	// Locales lists the locales to pick the accept-language from, as BCP 47
	// tags such as "de-DE". The model does not cover language; the pick is
	// uniform and seeded. Empty means "en-US".
	Locales []string `json:"locales,omitempty"`
}

// GeneratedIdentity is a BrowserIdentity together with the request language
// drawn for it.
type GeneratedIdentity struct {
	BrowserIdentity
	// Locale is the drawn locale tag.
	Locale string
	// AcceptLanguage is the accept-language value for Locale.
	AcceptLanguage string
	// ModelVersion labels the model data the identity was drawn from. The
	// same seed under another ModelVersion can give another identity, so a
	// caller that needs an identity to stay put must store it, not the seed.
	ModelVersion string
}

// RequestHeaders is BrowserIdentity.RequestHeaders with AcceptLanguage filled
// in from the drawn locale unless opts sets one.
func (g GeneratedIdentity) RequestHeaders(opts HeaderOptions) (RequestHeaders, error) {
	if opts.AcceptLanguage == "" {
		opts.AcceptLanguage = g.AcceptLanguage
	}
	return g.BrowserIdentity.RequestHeaders(opts)
}

// WeightedIdentity is an identity and its probability under the constraints.
type WeightedIdentity struct {
	Identity BrowserIdentity
	Weight   float64
}

// IdentitySeed turns a stable key (an account id, a session id) into a seed.
// It is the first eight bytes of the key's SHA-256, big-endian, so any
// language reproduces it.
func IdentitySeed(key string) uint64 {
	h := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(h[:8])
}

// GenerateIdentity draws one browser identity from a model of real browser
// traffic, restricted to identities a built-in TLS profile backs. The same
// seed and constraints always give the same identity under one ModelVersion.
//
// The model chooses which built-in entry (browser, major version, OS) is
// presented, in proportion to its share of observed traffic. It never supplies
// strings: the User-Agent, Client Hints and header values come from the
// built-in entry, so the captured Chrome 156 values stay authoritative.
//
// Draw once per session or account and keep the result; do not draw per
// request.
func GenerateIdentity(seed uint64, c Constraints) (GeneratedIdentity, error) {
	f, err := identitymodel.Default()
	if err != nil {
		return GeneratedIdentity{}, err
	}
	pool, err := newIdentityPool(c)
	if err != nil {
		return GeneratedIdentity{}, err
	}
	r := identitymodel.NewRand(seed)
	sample, ok := f.Networks["input"].Sample(r, pool.allow)
	if !ok {
		return GeneratedIdentity{}, ErrNoIdentity
	}
	bp, ok := pool.entry(sample)
	if !ok {
		return GeneratedIdentity{}, ErrNoIdentity
	}
	locales := c.Locales
	if len(locales) == 0 {
		locales = []string{"en-US"}
	}
	locale := locales[int(r.Float64()*float64(len(locales)))]
	al, err := acceptLanguageFor(locale)
	if err != nil {
		return GeneratedIdentity{}, err
	}
	return GeneratedIdentity{
		BrowserIdentity: BrowserIdentity{BrowserProfile: bp, ClientHints: ClientHintsHeaders(bp.UserAgent)},
		Locale:          locale,
		AcceptLanguage:  al,
		ModelVersion:    f.Source.Version(),
	}, nil
}

// IdentityDistribution returns every identity GenerateIdentity can draw under
// c and its exact probability, most likely first. It is the weight table for
// a caller that picks by its own stable scheme (for example rendezvous
// hashing, which keeps an assignment when the candidate list grows).
func IdentityDistribution(c Constraints) ([]WeightedIdentity, error) {
	f, err := identitymodel.Default()
	if err != nil {
		return nil, err
	}
	pool, err := newIdentityPool(c)
	if err != nil {
		return nil, err
	}
	acc := map[string]*WeightedIdentity{}
	for _, o := range f.Networks["input"].Distribution(pool.allow) {
		bp, ok := pool.entry(o.Values)
		if !ok {
			continue
		}
		k := string(bp.TLSProfile) + "|" + bp.Browser + "|" + bp.OS
		if w, ok := acc[k]; ok {
			w.Weight += o.P
			continue
		}
		acc[k] = &WeightedIdentity{
			Identity: BrowserIdentity{BrowserProfile: bp, ClientHints: ClientHintsHeaders(bp.UserAgent)},
			Weight:   o.P,
		}
	}
	out := make([]WeightedIdentity, 0, len(acc))
	for _, w := range acc {
		out = append(out, *w)
	}
	if len(out) == 0 {
		return nil, ErrNoIdentity
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		return out[i].Identity.UserAgent < out[j].Identity.UserAgent
	})
	return out, nil
}

// identityPool is the set of built-in entries the constraints allow, indexed
// by the model value they correspond to.
type identityPool struct {
	byCell map[poolCell]BrowserProfile
	cells  map[[2]string]bool // {os, device}
}

type poolCell struct {
	os, device, browserHTTP string
}

func newIdentityPool(c Constraints) (*identityPool, error) {
	profiles := map[TLSProfile]bool{}
	for _, p := range c.Profiles {
		profiles[p] = true
	}
	if len(c.Profiles) == 0 {
		for _, bp := range BuiltinProfiles {
			profiles[bp.TLSProfile] = true
		}
	}
	oses := map[string]bool{}
	for _, o := range c.OS {
		oses[strings.ToLower(o)] = true
	}
	p := &identityPool{byCell: map[poolCell]BrowserProfile{}, cells: map[[2]string]bool{}}
	for _, set := range [][]BrowserProfile{BuiltinProfiles, legacyProfiles} {
		for _, bp := range set {
			if !profiles[bp.TLSProfile] || (len(oses) > 0 && !oses[bp.OS]) {
				continue
			}
			if bp.Mobile && !c.IncludeMobile {
				continue
			}
			val, ok := modelBrowserValue(bp)
			if !ok {
				continue
			}
			device := "desktop"
			if bp.Mobile {
				device = "mobile"
			}
			k := poolCell{bp.OS, device, val}
			if _, dup := p.byCell[k]; dup {
				continue // first entry wins, as profileForTLS does
			}
			p.byCell[k] = bp
			p.cells[[2]string{bp.OS, device}] = true
		}
	}
	if len(p.byCell) == 0 {
		return nil, ErrNoIdentity
	}
	return p, nil
}

// allow is the constraint set for the model's input network: a value is
// permitted when at least one pool entry is reachable through it.
func (p *identityPool) allow(node string, partial map[string]string, value string) bool {
	switch node {
	case "*DEVICE":
		for k := range p.cells {
			if k[1] == value {
				return true
			}
		}
		return false
	case "*OPERATING_SYSTEM":
		return p.cells[[2]string{value, partial["*DEVICE"]}]
	case "*BROWSER_HTTP":
		_, ok := p.byCell[poolCell{partial["*OPERATING_SYSTEM"], partial["*DEVICE"], value}]
		return ok
	}
	return true
}

func (p *identityPool) entry(sample map[string]string) (BrowserProfile, bool) {
	bp, ok := p.byCell[poolCell{sample["*OPERATING_SYSTEM"], sample["*DEVICE"], sample["*BROWSER_HTTP"]}]
	return bp, ok
}

var (
	reFirefoxMajorMinor = regexp.MustCompile(`Firefox/(\d+\.\d+)`)
	reSafariVersionTok  = regexp.MustCompile(`Version/(\d+(?:\.\d+)*)`)
)

// modelBrowserValue renders a built-in entry as the model's *BROWSER_HTTP
// value ("chrome/156.0.0.0|2"). Only an exact version match counts: the model
// has no entry for a version the library has not measured, and an entry with
// no model value (Brave, which sends a Chrome User-Agent) is not drawn. The
// built-in TLS backends speak HTTP/2, hence "|2".
func modelBrowserValue(bp BrowserProfile) (string, bool) {
	ua := bp.UserAgent
	switch bp.Browser {
	case "chrome":
		return "chrome/" + ExtractChromeVersion(ua) + ".0.0.0|2", true
	case "edge":
		return "edge/" + extractEdgeVersion(ua) + ".0.0.0|2", true
	case "firefox":
		if m := reFirefoxMajorMinor.FindStringSubmatch(ua); m != nil {
			return "firefox/" + m[1] + "|2", true
		}
	case "safari":
		if m := reSafariVersionTok.FindStringSubmatch(ua); m != nil {
			return "safari/" + m[1] + "|2", true
		}
	}
	return "", false
}

var reLocaleTag = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// acceptLanguageFor renders Chrome's accept-language for a single-language
// browser: "de-DE" gives "de-DE,de;q=0.9", "de" gives "de".
func acceptLanguageFor(tag string) (string, error) {
	if !reLocaleTag.MatchString(tag) {
		return "", fmt.Errorf("stealth: invalid locale tag %q", tag)
	}
	lang, _, regional := strings.Cut(tag, "-")
	if !regional {
		return tag, nil
	}
	return tag + "," + lang + ";q=0.9", nil
}
