package identitymodel

import (
	"encoding/json"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
)

// SplitMix64 from seed 0: the published reference outputs, so the generator
// cannot drift from what other languages implement.
func TestRandKnownAnswer(t *testing.T) {
	r := NewRand(0)
	for i, want := range []uint64{0xe220a8397b1dcdaf, 0x6e789e6aa1b965f4, 0x06c45d188009454f} {
		if got := r.Uint64(); got != want {
			t.Fatalf("output %d = %#x, want %#x", i, got, want)
		}
	}
}

func TestDefaultModelIsValid(t *testing.T) {
	f, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if f.Source.Licence != "Apache-2.0" || f.Source.SHA256 == "" || (len(f.Source.Commit) != 40 && f.Source.Package == "") {
		t.Fatalf("incomplete provenance: %+v", f.Source)
	}
	nw := f.Networks["input"]
	want := []string{"*DEVICE", "*OPERATING_SYSTEM", "*BROWSER_HTTP", "*HTTP_VERSION", "*BROWSER"}
	var got []string
	for _, n := range nw.Nodes {
		got = append(got, n.Name)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("nodes = %v, want %v", got, want)
	}
}

func TestParseRejectsBrokenModels(t *testing.T) {
	good := `{"format":"identity-model/1","source":{},"networks":{"n":{"nodes":[{"name":"a","parents":[],"values":["x","y"],"cpt":[[0,0.5],[1,0.5]]}]}}}`
	if _, err := Parse([]byte(good)); err != nil {
		t.Fatalf("good model: %v", err)
	}
	for name, bad := range map[string]string{
		"sum":     strings.Replace(good, "[1,0.5]", "[1,0.4]", 1),
		"index":   strings.Replace(good, "[1,0.5]", "[7,0.5]", 1),
		"format":  strings.Replace(good, "identity-model/1", "other", 1),
		"parent":  strings.Replace(good, `"parents":[]`, `"parents":["ghost"]`, 1),
		"shallow": strings.Replace(good, `"parents":[]`, `"parents":["a"]`, 1),
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("%s: broken model accepted", name)
		}
	}
}

// toy: color -> shape. "red" has its own table, any other color falls to skip.
const toyModel = `{"format":"identity-model/1","source":{},"networks":{"n":{"nodes":[
 {"name":"color","parents":[],"values":["blue","green","red"],"cpt":[[0,0.2],[1,0.3],[2,0.5]]},
 {"name":"shape","parents":["color"],"values":["circle","square","star"],
  "cpt":{"d":{"red":[[0,0.1],[1,0.9]]},"s":[[0,0.5],[2,0.5]]}}]}}}`

func toyNet(t *testing.T) *Network {
	t.Helper()
	f, err := Parse([]byte(toyModel))
	if err != nil {
		t.Fatal(err)
	}
	return f.Networks["n"]
}

func TestDistributionToy(t *testing.T) {
	got := map[string]float64{}
	for _, o := range toyNet(t).Distribution(nil) {
		got[o.Values["color"]+"/"+o.Values["shape"]] = o.P
	}
	want := map[string]float64{
		"red/circle": 0.05, "red/square": 0.45,
		"blue/circle": 0.1, "blue/star": 0.1,
		"green/circle": 0.15, "green/star": 0.15, // skip table
	}
	for k, w := range want {
		if math.Abs(got[k]-w) > 1e-12 {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("outcomes = %v", got)
	}
}

// With circle forbidden under red the red branch keeps only square, and when
// every shape is forbidden for a color that color is dead and drops out.
func TestConstraintsAndBacktrackToy(t *testing.T) {
	nw := toyNet(t)
	allow := func(node string, p map[string]string, v string) bool {
		if node == "shape" && p["color"] == "blue" {
			return false
		}
		return !(node == "shape" && v == "circle" && p["color"] == "red")
	}
	got := map[string]float64{}
	for _, o := range nw.Distribution(allow) {
		got[o.Values["color"]] = got[o.Values["color"]] + o.P
	}
	if got["blue"] != 0 || math.Abs(got["red"]-0.5/0.8) > 1e-12 || math.Abs(got["green"]-0.3/0.8) > 1e-12 {
		t.Fatalf("colors = %v", got)
	}
	// Sample agrees, and never returns a forbidden or dead value.
	r := NewRand(7)
	n, red := 20000, 0
	for i := 0; i < n; i++ {
		s, ok := nw.Sample(r, allow)
		if !ok || s["color"] == "blue" || (s["color"] == "red" && s["shape"] == "circle") {
			t.Fatalf("bad sample %v ok=%v", s, ok)
		}
		if s["color"] == "red" {
			red++
		}
	}
	if p := float64(red) / float64(n); math.Abs(p-0.625) > 0.015 {
		t.Fatalf("P(red) = %v, want 0.625", p)
	}
}

func TestSampleUnsatisfiable(t *testing.T) {
	if _, ok := toyNet(t).Sample(NewRand(1), func(string, map[string]string, string) bool { return false }); ok {
		t.Fatal("sample under an unsatisfiable constraint")
	}
}

type reference struct {
	Samples   int `json:"samples"`
	Scenarios map[string]struct {
		Constraints map[string][]string `json:"constraints"`
		Counts      map[string]int      `json:"counts"`
	} `json:"scenarios"`
}

func cellKey(s map[string]string) string {
	return s["*DEVICE"] + "~" + s["*OPERATING_SYSTEM"] + "~" + s["*BROWSER_HTTP"]
}

// The oracle is the upstream implementation itself: apify_reference.json holds
// joint counts from 200k draws of generative-bayesian-network under the same
// constraints (testdata/oracle/apify_marginals.mjs). Our sampler's 10k draws
// and our exact distribution must agree with it to within sampling error.
func TestSamplerMatchesUpstream(t *testing.T) {
	raw, err := os.ReadFile("testdata/apify_reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref reference
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	f, _ := Default()
	nw := f.Networks["input"]
	const n = 10000
	for name, sc := range ref.Scenarios {
		t.Run(name, func(t *testing.T) {
			allow := func(node string, _ map[string]string, v string) bool {
				l, ok := sc.Constraints[node]
				return !ok || slices.Contains(l, v)
			}
			exact := map[string]float64{}
			for _, o := range nw.Distribution(allow) {
				exact[cellKey(o.Values)] += o.P
			}
			drawn := map[string]int{}
			r := NewRand(20261114)
			for i := 0; i < n; i++ {
				s, ok := nw.Sample(r, allow)
				if !ok {
					t.Fatal("no sample")
				}
				drawn[cellKey(s)]++
			}
			N := float64(ref.Samples)
			keys := map[string]bool{}
			for k := range sc.Counts {
				keys[k] = true
			}
			for k := range drawn {
				keys[k] = true
			}
			for k := range exact {
				keys[k] = true
			}
			for k := range keys {
				pRef := float64(sc.Counts[k]) / N
				// exact vs upstream: only the upstream sampling error.
				sigRef := math.Sqrt(pRef*(1-pRef)/N) + 1/N
				if d := math.Abs(exact[k] - pRef); d > 5*sigRef+2/N {
					t.Errorf("exact %s = %.5f, upstream %.5f", k, exact[k], pRef)
				}
				// our draws vs upstream: both sampling errors.
				pOur := float64(drawn[k]) / n
				sig := math.Sqrt(pRef*(1-pRef)/n+pRef*(1-pRef)/N) + 1/n
				if d := math.Abs(pOur - pRef); d > 5*sig+2.0/n {
					t.Errorf("drawn %s = %.5f, upstream %.5f (tol %.5f)", k, pOur, pRef, 5*sig)
				}
			}
		})
	}
}

func TestSampleIsDeterministic(t *testing.T) {
	f, _ := Default()
	nw := f.Networks["input"]
	for seed := uint64(0); seed < 50; seed++ {
		a, _ := nw.Sample(NewRand(seed), nil)
		b, _ := nw.Sample(NewRand(seed), nil)
		if cellKey(a) != cellKey(b) {
			t.Fatalf("seed %d: %v != %v", seed, a, b)
		}
	}
}

// Encode must round-trip through Parse without changing a probability, and
// keep one CPT subtree per line.
func TestEncodeRoundTrip(t *testing.T) {
	f, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := f.Encode()
	if err != nil {
		t.Fatal(err)
	}
	g, err := Parse(enc)
	if err != nil {
		t.Fatalf("encoded model does not parse: %v", err)
	}
	if g.Source != f.Source {
		t.Fatalf("source changed: %+v", g.Source)
	}
	a, b := f.Networks["input"].Distribution(nil), g.Networks["input"].Distribution(nil)
	if len(a) != len(b) {
		t.Fatalf("outcomes %d != %d", len(a), len(b))
	}
	for i := range a {
		if a[i].P != b[i].P || cellKey(a[i].Values) != cellKey(b[i].Values) {
			t.Fatalf("outcome %d changed", i)
		}
	}
	if lines := strings.Count(string(enc), "\n"); lines < 200 {
		t.Fatalf("only %d lines: the file is not laid out for review", lines)
	}
	if string(modelJSON) != string(enc) {
		t.Fatal("model.json is not in Encode's layout; regenerate it with cmd/identity-model-import")
	}
	again, _ := g.Encode()
	if string(again) != string(enc) {
		t.Fatal("Encode is not a fixed point")
	}
}
