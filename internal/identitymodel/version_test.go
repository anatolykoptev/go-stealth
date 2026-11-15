package identitymodel

import "testing"

func TestSourceVersion(t *testing.T) {
	for _, c := range []struct {
		s    Source
		want string
	}{
		{Source{Commit: "ec1345caa09acf1d78dc9c35a7d7addeecf1d63d", Committed: "2026-11-09"}, "2026-11-09-ec1345c"},
		{Source{Package: "header-generator@2.1.91", Committed: "2026-12-09"}, "2026-12-09-2.1.91"},
	} {
		if got := c.s.Version(); got != c.want {
			t.Errorf("Version() = %q, want %q", got, c.want)
		}
	}
}

func TestBrowserValuesSeesSkipAndDeeper(t *testing.T) {
	nw := toyNet(t)
	got := nw.BrowserValues("shape")
	for _, v := range []string{"circle", "square", "star"} {
		if !got[v] {
			t.Errorf("missing %s: %v", v, got)
		}
	}
}
