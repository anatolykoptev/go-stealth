package main

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"

	stealth "github.com/anatolykoptev/go-stealth"
	"github.com/anatolykoptev/go-stealth/internal/identitymodel"
)

// maxMajorsBehind is how far the model's newest Chrome may trail the newest
// Chrome in our built-in profiles. The upstream model once shipped with its
// newest Chrome 7 majors behind the world; a model that stale skews every
// draw toward old entries.
const maxMajorsBehind = 2

var reChromeValue = regexp.MustCompile(`^chrome/(\d+)\.0\.0\.0\|2$`)

// modelChromeMajors returns the Chrome majors (reduced User-Agent form over
// HTTP/2) the model has any probability for.
func modelChromeMajors(f *identitymodel.File) map[int]bool {
	out := map[int]bool{}
	for v := range f.Networks["input"].BrowserValues("*BROWSER_HTTP") {
		if m := reChromeValue.FindStringSubmatch(v); m != nil {
			n, _ := strconv.Atoi(m[1])
			out[n] = true
		}
	}
	return out
}

func shippedChromeMajors(profiles []stealth.BrowserProfile) []int {
	seen := map[int]bool{}
	for _, bp := range profiles {
		if bp.Browser != "chrome" {
			continue
		}
		if n, err := strconv.Atoi(stealth.ExtractChromeVersion(bp.UserAgent)); err == nil {
			seen[n] = true
		}
	}
	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// checkFresh refuses a model that cannot serve the built-in profiles: its
// newest Chrome is more than maxMajorsBehind behind the newest one we ship, or
// it has no entry for a Chrome major we ship.
func checkFresh(f *identitymodel.File, profiles []stealth.BrowserProfile) error {
	ours := shippedChromeMajors(profiles)
	if len(ours) == 0 {
		return fmt.Errorf("no Chrome profile to compare against")
	}
	have := modelChromeMajors(f)
	if len(have) == 0 {
		return fmt.Errorf("model has no Chrome entries")
	}
	newestModel, newestOurs := 0, ours[len(ours)-1]
	for n := range have {
		newestModel = max(newestModel, n)
	}
	if newestModel < newestOurs-maxMajorsBehind {
		return fmt.Errorf("model's newest Chrome is %d, more than %d majors behind our newest profile %d",
			newestModel, maxMajorsBehind, newestOurs)
	}
	var missing []int
	for _, n := range ours {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("model lacks the Chrome majors we ship: %v", missing)
	}
	return nil
}

// printSummary writes the shipped model's version, Chrome majors and the
// default identity distribution, one stable line each, for diffing.
func printSummary(w io.Writer) error {
	f, err := identitymodel.Default()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "model %s\n", f.Source.Version())
	var majors []int
	for n := range modelChromeMajors(f) {
		majors = append(majors, n)
	}
	sort.Ints(majors)
	fmt.Fprintf(w, "chrome majors %v\n", majors)
	dist, err := stealth.IdentityDistribution(stealth.Constraints{})
	if err != nil {
		return err
	}
	sort.Slice(dist, func(i, j int) bool { return dist[i].Identity.UserAgent < dist[j].Identity.UserAgent })
	for _, d := range dist {
		fmt.Fprintf(w, "%-10s %-7s %-8s %6.2f%%\n", d.Identity.TLSProfile, d.Identity.Browser, d.Identity.OS, 100*d.Weight)
	}
	return nil
}
