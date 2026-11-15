package stealth

import (
	"errors"

	"github.com/bogdanfinn/tls-client/profiles"
	tls "github.com/bogdanfinn/utls"
)

var errNoTrustAnchors = errors.New("stealth: base profile carries no trust_anchors extension to remove")

// trustAnchorsExtensionID is the TLS extension code point of trust_anchors
// (draft-ietf-tls-trust-anchor-ids). Google Chrome 156 sends it; Microsoft
// Edge 156 does not: the ja4_r of every Edge 156 capture lacks ca34 and
// the extension count is 16 against Chrome's 17.
const trustAnchorsExtensionID = 0xca34

// withoutTrustAnchors returns p with a ClientHello that omits the
// trust_anchors extension and is otherwise unchanged, which is Edge 156's
// ClientHello over Chrome 152's (the newest tls-client ships). It removes the
// extension whichever way the profile builds it, as a GenericExtension or as
// utls's own type, and fails the handshake when the profile has none: a base
// profile that no longer carries it would silently turn this into a no-op and
// present Chrome's hello as Edge's.
func withoutTrustAnchors(p profiles.ClientProfile) profiles.ClientProfile {
	id := p.GetClientHelloId()
	inner := id.SpecFactory
	id.SpecFactory = func() (tls.ClientHelloSpec, error) {
		spec, err := inner()
		if err != nil {
			return spec, err
		}
		kept := make([]tls.TLSExtension, 0, len(spec.Extensions))
		for _, e := range spec.Extensions {
			switch x := e.(type) {
			case *tls.GenericExtension:
				if x.Id == trustAnchorsExtensionID {
					continue
				}
			case *tls.TrustAnchorsExtension:
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == len(spec.Extensions) {
			return spec, errNoTrustAnchors
		}
		spec.Extensions = kept
		return spec, nil
	}
	return cloneProfile(p, id, p.GetHeaderPriority())
}
