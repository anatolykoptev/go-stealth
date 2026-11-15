package main

import (
	"fmt"
	"strings"
	"testing"

	stealth "github.com/anatolykoptev/go-stealth"
	"github.com/anatolykoptev/go-stealth/internal/identitymodel"
)

// modelWith builds a one-node model whose *BROWSER_HTTP knows exactly values.
func modelWith(t *testing.T, values ...string) *identitymodel.File {
	t.Helper()
	var idx, vals []string
	for i, v := range values {
		idx = append(idx, fmt.Sprintf("[%d,%v]", i, 1/float64(len(values))))
		vals = append(vals, fmt.Sprintf("%q", v))
	}
	doc := `{"format":"identity-model/1","source":{},"networks":{"input":{"nodes":[
	 {"name":"*BROWSER_HTTP","parents":[],"values":[` + strings.Join(vals, ",") + `],"cpt":[` + strings.Join(idx, ",") + `]}]}}}`
	f, err := identitymodel.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func chromeValues(majors ...int) []string {
	var out []string
	for _, m := range majors {
		out = append(out, fmt.Sprintf("chrome/%d.0.0.0|2", m))
	}
	return out
}

func TestShippedModelPassesFreshness(t *testing.T) {
	f, err := identitymodel.Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkFresh(f, stealth.BuiltinProfiles); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFreshRefusesStaleAndIncomplete(t *testing.T) {
	ours := shippedChromeMajors(stealth.BuiltinProfiles)
	if len(ours) < 2 {
		t.Fatalf("expected several shipped Chrome majors, got %v", ours)
	}
	newest := ours[len(ours)-1]
	cases := []struct {
		name    string
		values  []string
		wantErr string
	}{
		{"complete", chromeValues(ours...), ""},
		{"newer than ours", chromeValues(append(append([]int{}, ours...), newest+2)...), ""},
		// The stale upstream model of 2026-08: newest Chrome 143 when 150 was current.
		{"stale", chromeValues(131, 133, 143), "behind"},
		{"exactly the allowed lag but lacks majors", chromeValues(newest - maxMajorsBehind), "lacks"},
		{"missing one shipped major", chromeValues(ours[1:]...), "lacks"},
		{"no chrome", []string{"firefox/160.0|2"}, "no Chrome"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkFresh(modelWith(t, c.values...), stealth.BuiltinProfiles)
			switch {
			case c.wantErr == "" && err != nil:
				t.Fatalf("refused: %v", err)
			case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
				t.Fatalf("err = %v, want it to mention %q", err, c.wantErr)
			}
		})
	}
}
