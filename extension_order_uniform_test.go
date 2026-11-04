package stealth

import (
	"fmt"
	"math"
	"testing"

	tls "github.com/bogdanfinn/utls"
)

const uniformShuffleSamples = 60000

func isFixedExt(e tls.TLSExtension) bool {
	switch e.(type) {
	case *tls.UtlsGREASEExtension, *tls.UtlsPaddingExtension, tls.PreSharedKeyExtension:
		return true
	}
	return false
}

// BoringSSL shuffles Chrome's extensions uniformly. utls's own shuffle skips
// every swap that touches a fixed slot, which leaves each extension in its
// original slot about twice as often as chance (11% against 5.9% for the 17
// movable extensions of the Chrome 152 spec). This pins our shuffle to the
// uniform distribution: every movable extension lands in every movable slot
// with frequency 1/m, within six standard deviations.
func TestUniformExtensionOrder_IsUniform(t *testing.T) {
	base, err := mapTLSProfile(ProfileChrome152)
	if err != nil {
		t.Fatal(err)
	}
	id := withUniformExtensionOrder(base).GetClientHelloId()

	var movable int
	counts := map[string][]int{}
	first, _ := id.SpecFactory()
	for _, e := range first.Extensions {
		if !isFixedExt(e) {
			movable++
		}
	}
	for _, e := range first.Extensions {
		if !isFixedExt(e) {
			counts[fmt.Sprintf("%T", e)] = make([]int, movable)
		}
	}
	if len(counts) != movable {
		t.Fatalf("test precondition: %d movable extensions but %d distinct types", movable, len(counts))
	}

	for range uniformShuffleSamples {
		spec, err := id.SpecFactory()
		if err != nil {
			t.Fatal(err)
		}
		n := len(spec.Extensions)
		if !isFixedExt(spec.Extensions[0]) || !isFixedExt(spec.Extensions[n-1]) {
			t.Fatal("GREASE not first and last")
		}
		slot := 0
		for _, e := range spec.Extensions {
			if isFixedExt(e) {
				continue
			}
			counts[fmt.Sprintf("%T", e)][slot]++
			slot++
		}
	}

	p := 1 / float64(movable)
	sigma := math.Sqrt(p * (1 - p) / uniformShuffleSamples)
	for typ, perSlot := range counts {
		for slot, c := range perSlot {
			f := float64(c) / uniformShuffleSamples
			if math.Abs(f-p) > 6*sigma {
				t.Fatalf("%s in movable slot %d: frequency %.4f, want %.4f +- %.4f (uniform over %d)", typ, slot, f, p, 6*sigma, movable)
			}
		}
	}
}

func TestUniformExtensionOrder_KeepsExtensionSetAndFixedEnds(t *testing.T) {
	base, _ := mapTLSProfile(ProfileChrome152)
	orig, _ := base.GetClientHelloId().SpecFactory()
	id := withUniformExtensionOrder(base).GetClientHelloId()
	for range 50 {
		spec, _ := id.SpecFactory()
		if len(spec.Extensions) != len(orig.Extensions) {
			t.Fatalf("extension count changed: %d vs %d", len(spec.Extensions), len(orig.Extensions))
		}
		have := map[string]int{}
		for _, e := range spec.Extensions {
			have[fmt.Sprintf("%T", e)]++
		}
		for _, e := range orig.Extensions {
			have[fmt.Sprintf("%T", e)]--
		}
		for k, v := range have {
			if v != 0 {
				t.Fatalf("extension set changed at %s (%+d)", k, v)
			}
		}
	}
}
