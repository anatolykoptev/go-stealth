package stealth

import (
	"errors"
	"testing"
)

// HTTP/2 weights as echo services print them (wire byte + 1).
const (
	navWeight   = 256
	fetchWeight = 220
)

type recordedReq struct {
	headers map[string]string
}

// captureDoer is a backend that records the request headers it is handed.
type captureDoer struct {
	got *recordedReq
}

func (d captureDoer) Do(req *Request) (*Response, error) {
	h := make(map[string]string, len(req.Headers))
	for k, v := range req.Headers {
		h[k] = v
	}
	d.got.headers = h
	return &Response{StatusCode: 200, Headers: map[string]string{}}, nil
}
func (d captureDoer) SetProxy(string) error             { return nil }
func (d captureDoer) GetCookieValue(_, _ string) string { return "" }

func clientWithCapture(t *testing.T, opts ...ClientOption) (*BrowserClient, *recordedReq, *[]BackendConfig) {
	t.Helper()
	rec := &recordedReq{}
	cfgs := &[]BackendConfig{}
	factory := func(cfg BackendConfig) (HTTPDoer, error) {
		*cfgs = append(*cfgs, cfg)
		return captureDoer{got: rec}, nil
	}
	all := append([]ClientOption{WithBackend(factory), WithoutSSRFGuard()}, opts...)
	bc, err := NewClient(all...)
	if err != nil {
		t.Fatal(err)
	}
	return bc, rec, cfgs
}

func TestWire_FetchModeSendsFetchPriorityWeight(t *testing.T) {
	for _, p := range []TLSProfile{ProfileChrome152, ProfileChrome131, ProfileBrave146} {
		t.Run(string(p), func(t *testing.T) {
			w := captureWire(t, WithProfile(p), WithFetchMode())
			if w.Priority == nil {
				t.Fatal("HEADERS carried no priority")
			}
			if got := w.Priority.Weight(); got != fetchWeight || !w.Priority.Exclusive || w.Priority.DependsOn != 0 {
				t.Fatalf("fetch-mode priority = %+v (weight %d), want exclusive, dep 0, weight %d", *w.Priority, got, fetchWeight)
			}
		})
	}
}

// Navigation stays 256 unless the caller opts in; chrome_131 included.
func TestWire_NavigationKeepsWeight256(t *testing.T) {
	for _, p := range []TLSProfile{ProfileChrome152, ProfileChrome131} {
		t.Run(string(p), func(t *testing.T) {
			w := captureWire(t, WithProfile(p))
			if w.Priority == nil || w.Priority.Weight() != navWeight {
				t.Fatalf("default priority = %+v, want weight %d", w.Priority, navWeight)
			}
		})
	}
}

// Fetch mode swaps only the priority: the rest of the HTTP/2 preface must be
// the profile's own.
func TestWire_FetchModeChangesOnlyPriority(t *testing.T) {
	nav := captureWire(t, WithProfile(ProfileChrome152))
	fetch := captureWire(t, WithProfile(ProfileChrome152), WithFetchMode())
	if fetch.WindowUpdate != nav.WindowUpdate || len(fetch.Settings) != len(nav.Settings) {
		t.Fatalf("fetch-mode changed SETTINGS/WINDOW_UPDATE: %v/%d vs %v/%d", fetch.Settings, fetch.WindowUpdate, nav.Settings, nav.WindowUpdate)
	}
	for i := range nav.Settings {
		if fetch.Settings[i] != nav.Settings[i] {
			t.Fatalf("SETTINGS[%d] differs: %v vs %v", i, fetch.Settings[i], nav.Settings[i])
		}
	}
}

func TestFetchMode_AddsPriorityHeader(t *testing.T) {
	bc, rec, cfgs := clientWithCapture(t, WithFetchMode())
	caller := map[string]string{"accept": "*/*"}
	if _, _, _, err := bc.Do("GET", "https://example.com/", caller, nil); err != nil {
		t.Fatal(err)
	}
	if got := rec.headers["priority"]; got != "u=1, i" {
		t.Fatalf("priority header = %q, want %q", got, "u=1, i")
	}
	if _, polluted := caller["priority"]; polluted {
		t.Fatal("caller's header map was mutated")
	}
	if !(*cfgs)[0].FetchMode {
		t.Fatal("BackendConfig.FetchMode not set")
	}
}

func TestFetchMode_KeepsCallerPriority(t *testing.T) {
	for _, key := range []string{"priority", "Priority"} {
		bc, rec, _ := clientWithCapture(t, WithFetchMode())
		if _, _, _, err := bc.Do("GET", "https://example.com/", map[string]string{key: "u=3"}, nil); err != nil {
			t.Fatal(err)
		}
		if len(rec.headers) != 1 || rec.headers[key] != "u=3" {
			t.Fatalf("key %q: headers = %v, want the caller's priority only", key, rec.headers)
		}
	}
}

func TestFetchMode_OffAddsNoPriorityHeader(t *testing.T) {
	bc, rec, cfgs := clientWithCapture(t)
	if _, _, _, err := bc.Do("GET", "https://example.com/", map[string]string{"accept": "*/*"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := rec.headers["priority"]; ok {
		t.Fatalf("priority header added without WithFetchMode: %v", rec.headers)
	}
	if (*cfgs)[0].FetchMode {
		t.Fatal("BackendConfig.FetchMode set without WithFetchMode")
	}
}

func TestFetchMode_RejectsNonChromeProfiles(t *testing.T) {
	for _, p := range []TLSProfile{ProfileFirefox148, ProfileSafariIOS26} {
		_, err := NewClient(WithProfile(p), WithFetchMode())
		if !errors.Is(err, ErrFetchModeUnsupported) {
			t.Fatalf("%s: err = %v, want ErrFetchModeUnsupported", p, err)
		}
	}
	for _, p := range []TLSProfile{ProfileChrome152, ProfileChrome131, ProfileBrave146} {
		if _, err := NewClient(WithProfile(p), WithFetchMode()); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
}
