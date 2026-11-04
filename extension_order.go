package stealth

import (
	"math/rand/v2"

	"github.com/bogdanfinn/fhttp/http2"
	"github.com/bogdanfinn/tls-client/profiles"
	tls "github.com/bogdanfinn/utls"
)

// withUniformExtensionOrder returns p with a ClientHello that shuffles its
// extensions afresh for every connection, the way BoringSSL does for Chrome:
// every extension except the GREASE ones (first and last), padding and
// pre_shared_key is placed by a uniform random permutation.
//
// It replaces tls-client's WithRandomTLSExtensionOrder, which calls utls's
// ShuffleChromeTLSExtensions. That function swaps pairs of positions and skips
// a swap whenever either position is a fixed extension, so a movable extension
// stays in its original slot about twice as often as chance (11% against 5.9%
// for the 17 movable extensions of the Chrome 152 spec, measured over 200k
// runs). The distribution is pinned by TestUniformExtensionOrder_IsUniform.
func withUniformExtensionOrder(p profiles.ClientProfile) profiles.ClientProfile {
	id := p.GetClientHelloId()
	inner := id.SpecFactory
	id.SpecFactory = func() (tls.ClientHelloSpec, error) {
		spec, err := inner()
		if err != nil {
			return spec, err
		}
		shuffleMovableExtensions(spec.Extensions)
		return spec, nil
	}
	return cloneProfile(p, id, p.GetHeaderPriority())
}

// shuffleMovableExtensions permutes the movable extensions of exts in place
// (Fisher-Yates over their positions) and leaves the fixed ones where they are.
func shuffleMovableExtensions(exts []tls.TLSExtension) {
	pos := make([]int, 0, len(exts))
	for i, e := range exts {
		switch e.(type) {
		case *tls.UtlsGREASEExtension, *tls.UtlsPaddingExtension, tls.PreSharedKeyExtension:
		default:
			pos = append(pos, i)
		}
	}
	for i := len(pos) - 1; i > 0; i-- {
		j := rand.IntN(i + 1)
		exts[pos[i]], exts[pos[j]] = exts[pos[j]], exts[pos[i]]
	}
}

// cloneProfile rebuilds p with the given ClientHello and HEADERS priority and
// every other setting carried over untouched.
func cloneProfile(p profiles.ClientProfile, id tls.ClientHelloID, prio *http2.PriorityParam) profiles.ClientProfile {
	return profiles.NewClientProfile(
		id, p.GetSettings(), p.GetSettingsOrder(), p.GetPseudoHeaderOrder(),
		p.GetConnectionFlow(), p.GetPriorities(), prio,
		p.GetStreamID(), p.GetAllowHTTP(),
		p.GetHttp3Settings(), p.GetHttp3SettingsOrder(), p.GetHttp3PriorityParam(),
		p.GetHttp3PseudoHeaderOrder(), p.GetHttp3SendGreaseFrames(),
	)
}
