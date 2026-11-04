package stealth

import (
	"slices"
	"testing"
)

var wantNavStable = []string{
	"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
	"upgrade-insecure-requests", "user-agent", "accept",
	"sec-fetch-site", "sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest",
	"referer", "accept-encoding", "accept-language", "cookie", "priority",
}

func TestChromeNavigationHeaderOrder_MatchesStableChrome156(t *testing.T) {
	if !slices.Equal(ChromeNavigationHeaderOrder, wantNavStable) {
		t.Fatalf("got  %v\nwant %v", ChromeNavigationHeaderOrder, wantNavStable)
	}
}

func TestChromeFetchHeaderOrder(t *testing.T) {
	std := []string{
		"sec-ch-ua-platform", "sec-ch-ua", "sec-ch-ua-mobile", "user-agent", "accept",
		"sec-fetch-site", "sec-fetch-mode", "sec-fetch-dest", "referer",
		"accept-encoding", "accept-language", "cookie", "priority",
	}
	if got := ChromeFetchHeaderOrder(nil); !slices.Equal(got, std) {
		t.Fatalf("no custom headers:\n got  %v\n want %v", got, std)
	}
	custom := []string{"authorization", "x-csrf-token"}
	got := ChromeFetchHeaderOrder(custom)
	want := append(append([]string{"sec-ch-ua-platform"}, custom...), std[1:]...)
	if !slices.Equal(got, want) {
		t.Fatalf("custom block placement:\n got  %v\n want %v", got, want)
	}
	// The result must not alias the caller's slice or any shared state.
	got[1] = "mutated"
	if custom[0] != "authorization" || ChromeFetchHeaderOrder(custom)[1] != "authorization" {
		t.Fatal("ChromeFetchHeaderOrder shares memory with its argument or a package variable")
	}
}

func orderOf(t *testing.T, opts ...ClientOption) []string {
	t.Helper()
	bc, rec, _ := clientWithCapture(t, opts...)
	if _, _, _, err := bc.Do("GET", "https://example.com/", map[string]string{"accept": "*/*"}, nil); err != nil {
		t.Fatal(err)
	}
	return rec.order
}

// The default client is Chrome 156, so it carries Chrome 156's order; the other
// Chrome profiles keep the order they always had.
func TestClientHeaderOrder_PerProfile(t *testing.T) {
	if got := orderOf(t); !slices.Equal(got, ChromeNavigationHeaderOrder) {
		t.Errorf("default client order = %v, want ChromeNavigationHeaderOrder", got)
	}
	for _, p := range []TLSProfile{ProfileChrome152, ProfileChrome131} {
		if got := orderOf(t, WithProfile(p)); !slices.Equal(got, DefaultHeaderOrder) {
			t.Errorf("%s order = %v, want DefaultHeaderOrder", p, got)
		}
	}
	if got := orderOf(t, WithProfile(ProfileFirefox148)); !slices.Equal(got, FirefoxHeaderOrder) {
		t.Errorf("firefox order = %v", got)
	}
	mine := []string{"user-agent", "accept"}
	if got := orderOf(t, WithHeaderOrder(mine)); !slices.Equal(got, mine) {
		t.Errorf("WithHeaderOrder not honoured: %v", got)
	}
}

func TestClientHeaderOrder_FetchModeDefault(t *testing.T) {
	if got := orderOf(t, WithFetchMode()); !slices.Equal(got, ChromeFetchHeaderOrder(nil)) {
		t.Errorf("fetch-mode order = %v, want ChromeFetchHeaderOrder(nil)", got)
	}
	mine := []string{"accept", "user-agent"}
	if got := orderOf(t, WithFetchMode(), WithHeaderOrder(mine)); !slices.Equal(got, mine) {
		t.Errorf("explicit WithHeaderOrder must win over the fetch default: %v", got)
	}
}
