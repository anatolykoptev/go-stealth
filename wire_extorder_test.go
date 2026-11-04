package stealth

import (
	"fmt"
	"slices"
	"testing"
)

const extOrderSamples = 12

// modernRandomisedProfiles are the Chrome-family profiles that must shuffle
// their TLS extension order per connection, as Chrome (106+) does. Brave is in
// the set: its tls-client profile carries RandomExtensionOrder but the flag
// is not honoured, so without the explicit client option its order is fixed.
var modernRandomisedProfiles = []TLSProfile{ProfileChrome146, ProfileChrome150, ProfileChrome152, ProfileChrome156, ProfileBrave146}

// frozenOrderProfiles keep the extension order tls-client ships. chrome_131 is
// presented on purpose by existing consumers and its wire must not move;
// 133 and 144 are legacy and stay as they were.
var frozenOrderProfiles = []TLSProfile{ProfileChrome131, ProfileChrome133, ProfileChrome144}

func extOrders(t *testing.T, p TLSProfile) [][]uint16 {
	t.Helper()
	orders := make([][]uint16, 0, extOrderSamples)
	for range extOrderSamples {
		orders = append(orders, captureWire(t, WithProfile(p)).Extensions)
	}
	return orders
}

func distinctOrders(orders [][]uint16) int {
	seen := map[string]bool{}
	for _, o := range orders {
		seen[fmt.Sprint(o)] = true
	}
	return len(seen)
}

// A fixed extension order is a one-JA3 fingerprint that no real Chrome has.
func TestWire_ModernProfilesRandomiseExtensionOrder(t *testing.T) {
	for _, p := range modernRandomisedProfiles {
		t.Run(string(p), func(t *testing.T) {
			orders := extOrders(t, p)
			if n := distinctOrders(orders); n < 2 {
				t.Fatalf("%s: %d distinct extension orders over %d connections, want a different order per connection", p, n, extOrderSamples)
			}
			ref := slices.Clone(orders[0])
			slices.Sort(ref)
			for i, o := range orders {
				// Shuffling must permute, never add or drop, and keep GREASE at both ends.
				got := slices.Clone(o)
				slices.Sort(got)
				if !slices.Equal(got, ref) {
					t.Fatalf("%s conn %d: extension set changed: %x vs %x", p, i, got, ref)
				}
				if o[0] != greaseNormalised || o[len(o)-1] != greaseNormalised {
					t.Fatalf("%s conn %d: GREASE not first and last: %x", p, i, o)
				}
			}
		})
	}
}

func TestWire_LegacyProfilesKeepFixedExtensionOrder(t *testing.T) {
	for _, p := range frozenOrderProfiles {
		t.Run(string(p), func(t *testing.T) {
			if n := distinctOrders(extOrders(t, p)); n != 1 {
				t.Fatalf("%s: %d distinct extension orders, want exactly 1 (frozen wire)", p, n)
			}
		})
	}
}
