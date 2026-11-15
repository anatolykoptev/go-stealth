package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"

	stealth "github.com/anatolykoptev/go-stealth"
)

// poolFile is what the oracle script reads: the desktop OSes and *BROWSER_HTTP
// values of every identity the default constraints can reach with the weight
// floor off.
type poolFile struct {
	DesktopOS   []string `json:"desktop_os"`
	BrowserHTTP []string `json:"browser_http"`
}

// browserHTTPValue renders an identity as the model's *BROWSER_HTTP value. Only
// the browsers the model can tell apart and the library ships are handled; a
// new one stops the generator rather than being left out of the oracle.
func browserHTTPValue(bp stealth.BrowserProfile) (string, error) {
	switch bp.Browser {
	case "chrome":
		return "chrome/" + stealth.ExtractChromeVersion(bp.UserAgent) + ".0.0.0|2", nil
	case "edge":
		return "edge/" + edgeMajor(bp.UserAgent) + ".0.0.0|2", nil
	}
	return "", fmt.Errorf("default pool contains a %s entry; teach browserHTTPValue about it", bp.Browser)
}

var reEdge = regexp.MustCompile(`Edg/(\d+)\.`)

func edgeMajor(ua string) string {
	if m := reEdge.FindStringSubmatch(ua); m != nil {
		return m[1]
	}
	return ""
}

func buildPoolFile() (poolFile, error) {
	dist, err := stealth.IdentityDistribution(stealth.Constraints{MinWeight: -1})
	if err != nil {
		return poolFile{}, err
	}
	oses, vals := map[string]bool{}, map[string]bool{}
	for _, d := range dist {
		v, err := browserHTTPValue(d.Identity.BrowserProfile)
		if err != nil {
			return poolFile{}, err
		}
		oses[d.Identity.OS] = true
		vals[v] = true
	}
	var out poolFile
	for k := range oses {
		out.DesktopOS = append(out.DesktopOS, k)
	}
	for k := range vals {
		out.BrowserHTTP = append(out.BrowserHTTP, k)
	}
	sort.Strings(out.DesktopOS)
	sort.Strings(out.BrowserHTTP)
	return out, nil
}

func writePool(path string) error {
	p, err := buildPoolFile()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
