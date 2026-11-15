package stealth

import (
	"encoding/json"
	"errors"
	"flag"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

var updateIdentityVectors = flag.Bool("update-identity-vectors", false, "rewrite testdata/identity_model_vectors.json")

func constraintSets() map[string]Constraints {
	return map[string]Constraints{
		"default":    {},
		"mobile":     {IncludeMobile: true},
		"linux":      {OS: []string{"linux"}},
		"only156":    {Profiles: []TLSProfile{ProfileChrome156}},
		"locales":    {Locales: []string{"en-US", "de-DE", "fr-FR", "ja-JP"}},
		"legacy_131": {Profiles: []TLSProfile{ProfileChrome131}},
	}
}

func TestGenerateIdentity_BackedByBuiltinProfile(t *testing.T) {
	platform := map[string]string{"windows": `"Windows"`, "macos": `"macOS"`, "linux": `"Linux"`, "android": `"Android"`}
	for name, c := range constraintSets() {
		t.Run(name, func(t *testing.T) {
			for seed := uint64(0); seed < 3000; seed++ {
				g, err := GenerateIdentity(seed, c)
				if err != nil {
					t.Fatalf("seed %d: %v", seed, err)
				}
				var entry *BrowserProfile
				for _, set := range [][]BrowserProfile{BuiltinProfiles, legacyProfiles} {
					for i := range set {
						if set[i] == g.BrowserProfile {
							entry = &set[i]
						}
					}
				}
				if entry == nil {
					t.Fatalf("seed %d: %+v is not a built-in profile entry", seed, g.BrowserProfile)
				}
				if len(c.Profiles) > 0 && !containsProfile(c.Profiles, g.TLSProfile) {
					t.Fatalf("seed %d: profile %s outside the constraint", seed, g.TLSProfile)
				}
				if len(c.OS) > 0 && g.OS != c.OS[0] {
					t.Fatalf("seed %d: OS %s outside the constraint", seed, g.OS)
				}
				if g.Mobile && !c.IncludeMobile {
					t.Fatalf("seed %d: mobile identity without IncludeMobile", seed)
				}
				// Coherence, as TestBuiltinProfilesCoherent pins for the table.
				if want := ClientHintsHeaders(g.UserAgent); !reflect.DeepEqual(g.ClientHints, want) {
					t.Fatalf("seed %d: client hints %v, want %v", seed, g.ClientHints, want)
				}
				if got, major := ExtractChromeVersion(g.UserAgent), firstNumericToken(g.TLSProfile); got != major {
					t.Fatalf("seed %d: UA major %s over TLS profile %s", seed, got, g.TLSProfile)
				}
				if g.ClientHints["sec-ch-ua-platform"] != platform[g.OS] {
					t.Fatalf("seed %d: platform hint %s for OS %s", seed, g.ClientHints["sec-ch-ua-platform"], g.OS)
				}
				if (g.ClientHints["sec-ch-ua-mobile"] == "?1") != g.Mobile {
					t.Fatalf("seed %d: mobile hint %s for Mobile=%v", seed, g.ClientHints["sec-ch-ua-mobile"], g.Mobile)
				}
				if _, err := g.RequestHeaders(HeaderOptions{}); err != nil {
					t.Fatalf("seed %d: %v", seed, err)
				}
			}
		})
	}
}

func containsProfile(l []TLSProfile, p TLSProfile) bool {
	for _, x := range l {
		if x == p {
			return true
		}
	}
	return false
}

func TestGenerateIdentity_SameSeedSameIdentity(t *testing.T) {
	c := Constraints{IncludeMobile: true, Locales: []string{"en-US", "de-DE", "fr-FR"}}
	distinct := map[string]bool{}
	for seed := uint64(0); seed < 2000; seed++ {
		a, _ := GenerateIdentity(seed, c)
		b, _ := GenerateIdentity(seed, c)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("seed %d gave two identities", seed)
		}
		distinct[a.UserAgent+"|"+a.AcceptLanguage] = true
	}
	if len(distinct) < 8 {
		t.Fatalf("only %d distinct identities in 2000 seeds", len(distinct))
	}
	if IdentitySeed("account-1") == IdentitySeed("account-2") || IdentitySeed("account-1") != IdentitySeed("account-1") {
		t.Fatal("IdentitySeed is not a stable function of the key")
	}
}

// 10k draws against the exact distribution the model gives for the same
// constraints.
func TestGenerateIdentity_MarginalsFollowModel(t *testing.T) {
	dist, err := IdentityDistribution(Constraints{})
	if err != nil {
		t.Fatal(err)
	}
	var sum float64
	want := map[string]float64{}
	for _, w := range dist {
		sum += w.Weight
		want[w.Identity.UserAgent] = w.Weight
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("weights sum to %v", sum)
	}
	const n = 10000
	got := map[string]int{}
	for seed := uint64(0); seed < n; seed++ {
		g, err := GenerateIdentity(IdentitySeed(string(rune('a'+seed%26))+itoa(seed)), Constraints{})
		if err != nil {
			t.Fatal(err)
		}
		got[g.UserAgent]++
	}
	for ua, p := range want {
		sig := math.Sqrt(p*(1-p)/n) + 1.0/n
		if d := math.Abs(float64(got[ua])/n - p); d > 5*sig+2.0/n {
			t.Errorf("%s: drawn %.4f, model %.4f", ua, float64(got[ua])/n, p)
		}
	}
	for ua := range got {
		if _, ok := want[ua]; !ok {
			t.Errorf("drew %s, which the distribution does not contain", ua)
		}
	}
}

func itoa(u uint64) string {
	b, _ := json.Marshal(u)
	return string(b)
}

// The OS mix at the public layer against the upstream implementation's own
// counts (internal/identitymodel/testdata/apify_reference.json): same
// constraints apart from the entries no built-in profile backs.
func TestIdentityDistribution_OSMixMatchesUpstream(t *testing.T) {
	raw, err := os.ReadFile("internal/identitymodel/testdata/apify_reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Samples   float64 `json:"samples"`
		Scenarios map[string]struct {
			Counts map[string]float64 `json:"counts"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	wantOS := map[string]float64{}
	for k, c := range ref.Scenarios["desktop_chromium"].Counts {
		wantOS[strings.Split(k, "~")[1]] += c / ref.Samples
	}
	dist, err := IdentityDistribution(Constraints{})
	if err != nil {
		t.Fatal(err)
	}
	gotOS := map[string]float64{}
	for _, w := range dist {
		gotOS[w.Identity.OS] += w.Weight
	}
	for o, p := range wantOS {
		if math.Abs(gotOS[o]-p) > 0.01 {
			t.Errorf("%s share %.4f, upstream %.4f", o, gotOS[o], p)
		}
	}
}

// For Chrome 156 the captured, branded values win: every drawn identity on the
// Windows entry produces the same header set as DefaultIdentity, and a drawn
// identity on another OS differs from it only where the OS does.
func TestGenerateIdentity_Chrome156CanonWins(t *testing.T) {
	def := DefaultIdentity()
	if def.TLSProfile != ProfileChrome156 || def.OS != "windows" {
		t.Fatalf("default identity is %s/%s; this test pins the Windows Chrome 156 capture", def.TLSProfile, def.OS)
	}
	seen := map[string]bool{}
	for seed := uint64(0); seed < 4000; seed++ {
		g, err := GenerateIdentity(seed, Constraints{Profiles: []TLSProfile{ProfileChrome156}})
		if err != nil {
			t.Fatal(err)
		}
		seen[g.OS] = true
		for _, opts := range []HeaderOptions{{}, {Mode: FetchCORS}} {
			got, err := g.RequestHeaders(opts)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := def.RequestHeaders(opts)
			if g.OS == "windows" {
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("windows chrome 156 headers differ from the capture:\n got %v\nwant %v", got, want)
				}
				continue
			}
			for k, v := range want.Values {
				if k == "user-agent" || k == "sec-ch-ua-platform" {
					continue
				}
				if got.Values[k] != v {
					t.Fatalf("%s %s = %q, want the captured %q", g.OS, k, got.Values[k], v)
				}
			}
			if !reflect.DeepEqual(got.Order, want.Order) {
				t.Fatalf("%s header order %v, captured %v", g.OS, got.Order, want.Order)
			}
		}
	}
	if !seen["windows"] || !seen["macos"] || !seen["linux"] {
		t.Fatalf("OS variety missing: %v", seen)
	}
}

// chrome_131 is frozen: the sampler never reaches it unless asked, and when
// asked it returns the table entry unchanged.
func TestGenerateIdentity_FrozenProfilesNeverDrawnByDefault(t *testing.T) {
	dist, _ := IdentityDistribution(Constraints{IncludeMobile: true})
	for _, w := range dist {
		switch w.Identity.TLSProfile {
		case ProfileChrome131, ProfileChrome133, ProfileChrome144:
			t.Fatalf("default pool contains %s", w.Identity.TLSProfile)
		}
	}
	for seed := uint64(0); seed < 500; seed++ {
		g, err := GenerateIdentity(seed, Constraints{Profiles: []TLSProfile{ProfileChrome131}})
		if err != nil {
			t.Fatal(err)
		}
		if g.TLSProfile != ProfileChrome131 || !strings.Contains(g.UserAgent, "Chrome/131.0.0.0") {
			t.Fatalf("seed %d: %+v", seed, g.BrowserProfile)
		}
	}
}

func TestGenerateIdentity_Errors(t *testing.T) {
	for name, c := range map[string]Constraints{
		"no desktop ios":     {OS: []string{"ios"}},
		"ios not in model":   {OS: []string{"ios"}, IncludeMobile: true},
		"unknown profile":    {Profiles: []TLSProfile{"chrome_999"}},
		"os without entries": {OS: []string{"plan9"}},
	} {
		if _, err := GenerateIdentity(1, c); !errors.Is(err, ErrNoIdentity) {
			t.Errorf("%s: err = %v, want ErrNoIdentity", name, err)
		}
	}
	if _, err := GenerateIdentity(1, Constraints{Locales: []string{"not a locale"}}); err == nil {
		t.Error("invalid locale accepted")
	}
}

func TestAcceptLanguageFor(t *testing.T) {
	for tag, want := range map[string]string{
		"en-US":      "en-US,en;q=0.9",
		"de-DE":      "de-DE,de;q=0.9",
		"de":         "de",
		"zh-Hant-TW": "zh-Hant-TW,zh;q=0.9",
	} {
		if got, err := acceptLanguageFor(tag); err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", tag, got, err, want)
		}
	}
	g, _ := GenerateIdentity(5, Constraints{Locales: []string{"de-DE"}})
	rh, _ := g.RequestHeaders(HeaderOptions{})
	if rh.Values["accept-language"] != "de-DE,de;q=0.9" {
		t.Errorf("accept-language = %q", rh.Values["accept-language"])
	}
	rh, _ = g.RequestHeaders(HeaderOptions{AcceptLanguage: "fr"})
	if rh.Values["accept-language"] != "fr" {
		t.Errorf("explicit AcceptLanguage lost: %q", rh.Values["accept-language"])
	}
}

type identityVector struct {
	Name           string      `json:"name"`
	Seed           uint64      `json:"seed"`
	Constraints    Constraints `json:"constraints"`
	TLSProfile     string      `json:"tls_profile"`
	Browser        string      `json:"browser"`
	OS             string      `json:"os"`
	UserAgent      string      `json:"user_agent"`
	Locale         string      `json:"locale"`
	AcceptLanguage string      `json:"accept_language"`
}

// The vectors pin seed -> identity for the shipped model. They are also the
// cross-language contract: a second implementation of the sampler must
// reproduce every row. Regenerate with -update-identity-vectors when the model
// data or the built-in profile set changes, and review the diff: each changed
// row is an identity that would move for a caller who re-derives from a seed.
func TestGenerateIdentity_Vectors(t *testing.T) {
	const path = "testdata/identity_model_vectors.json"
	build := func() []identityVector {
		var out []identityVector
		for name, c := range map[string]Constraints{
			"default": {},
			"mobile":  {IncludeMobile: true, Locales: []string{"en-US", "de-DE", "ja-JP"}},
			"mac":     {OS: []string{"macos"}, Locales: []string{"fr-FR"}},
		} {
			for _, seed := range []uint64{0, 1, 2, 3, 4, 5, 6, 7, 42, 1000, 1 << 40, IdentitySeed("account-1")} {
				g, err := GenerateIdentity(seed, c)
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, identityVector{name, seed, c, string(g.TLSProfile), g.Browser, g.OS, g.UserAgent, g.Locale, g.AcceptLanguage})
			}
		}
		sortVectors(out)
		return out
	}
	got := build()
	if *updateIdentityVectors {
		b, _ := json.MarshalIndent(got, "", " ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want []identityVector
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("seed -> identity changed; if intended, rerun with -update-identity-vectors and review the diff")
	}
}

func sortVectors(v []identityVector) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && (v[j].Name < v[j-1].Name || (v[j].Name == v[j-1].Name && v[j].Seed < v[j-1].Seed)); j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func TestMinWeightFloor(t *testing.T) {
	check := func(name string, c Constraints, floor float64) []WeightedIdentity {
		t.Helper()
		d, err := IdentityDistribution(c)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var sum float64
		for _, w := range d {
			sum += w.Weight
			if floor > 0 && w.Weight < floor {
				t.Errorf("%s: %s/%s %s has weight %.4f under the %.2f floor", name, w.Identity.TLSProfile, w.Identity.Browser, w.Identity.OS, w.Weight, floor)
			}
		}
		if math.Abs(sum-1) > 1e-9 {
			t.Errorf("%s: weights sum to %v, want renormalised to 1", name, sum)
		}
		return d
	}
	def := check("default", Constraints{}, DefaultMinWeight)
	off := check("off", Constraints{MinWeight: -1}, 0)
	high := check("5%", Constraints{MinWeight: 0.05}, 0.05)
	if len(off) <= len(def) || len(def) <= len(high) {
		t.Fatalf("floor should shrink the pool: off=%d default=%d 5%%=%d", len(off), len(def), len(high))
	}
	// The tail the default floor drops is never drawn.
	kept := map[string]bool{}
	for _, w := range def {
		kept[w.Identity.UserAgent] = true
	}
	for seed := uint64(0); seed < 5000; seed++ {
		g, err := GenerateIdentity(seed, Constraints{})
		if err != nil {
			t.Fatal(err)
		}
		if !kept[g.UserAgent] {
			t.Fatalf("seed %d drew %s, below the floor", seed, g.UserAgent)
		}
	}
	// With the floor off the tail is reachable.
	tail := map[string]bool{}
	for _, w := range off {
		if !kept[w.Identity.UserAgent] {
			tail[w.Identity.UserAgent] = true
		}
	}
	reached := false
	for seed := uint64(0); seed < 20000 && !reached; seed++ {
		g, _ := GenerateIdentity(seed, Constraints{MinWeight: -1})
		reached = tail[g.UserAgent]
	}
	if !reached {
		t.Fatal("floor off never drew a tail identity")
	}
	// A floor above 1 leaves nothing.
	if _, err := GenerateIdentity(1, Constraints{MinWeight: 1.5}); !errors.Is(err, ErrNoIdentity) {
		t.Errorf("floor above every weight: err = %v, want ErrNoIdentity", err)
	}
}
