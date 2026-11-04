//go:build wirelive

// Live wire check: measures what each go-stealth Chrome profile actually
// presents, through tls.peet.ws, and compares it field by field with captures
// of a real browser taken through the same service (testdata/wire). It needs
// the network and is NOT part of `make preflight`:
//
//	make fingerprint-wire
//
// An unreachable endpoint is a FAILURE, never a skip: a skipped check looks
// identical to a passing one. A mismatch is a true result; fix the profile in
// a reviewed change rather than loosening a comparison.
//
// Reference per profile (the reference major must equal the profile's, so
// chrome_131 is not compared with a Chrome 156 capture):
//
//	chrome_156, default   Google Chrome 156.0.8078.80 (stable, branded)
//	chrome_152            the same Google Chrome 156 capture: the claim under
//	                      test is that Chrome_152's wire equals Chrome 156's
//	chrome_131            pinned to its own recorded wire; the Chrome for
//	                      Testing 131 capture is shown for information only,
//	                      because tls-client's contributed Chrome_131 differs
//	                      from it (old ALPS codepoint 0x4469, no GREASE
//	                      SETTINGS entry) and consumers present it as it is
//
// GREASE values and the extension order change per connection by design:
// JA4 and peetprint sort or drop them, so they are compared exactly, while JA3
// is checked only for whether it varies across connections.
package stealth

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	liveEndpoint   = "https://tls.peet.ws/api/all"
	liveSamples    = 6 // fresh connections per profile, to see whether JA3 varies
	liveAttempts   = 3
	liveRetryPause = 4 * time.Second
)

type liveRef struct {
	Provenance struct {
		Browser    string `json:"browser"`
		Version    string `json:"version"`
		Mode       string `json:"mode"`
		CapturedAt string `json:"captured_at"`
	}
	TLS struct {
		JA4           string `json:"ja4"`
		JA4R          string `json:"ja4_r"`
		PeetprintHash string `json:"peetprint_hash"`
	}
	HTTP2 struct {
		Akamai   string `json:"akamai_fingerprint"`
		Priority struct {
			Weight    int
			Exclusive bool
			DependsOn int `json:"depends_on"`
		}
		RequestHeaderNames []string `json:"request_header_names"`
		PriorityHeader     string   `json:"priority_header"`
	}
}

func loadLiveRef(t *testing.T, name string) liveRef {
	t.Helper()
	b, err := os.ReadFile("testdata/wire/" + name)
	if err != nil {
		t.Fatalf("no reference %s (a missing reference fails, it does not skip): %v", name, err)
	}
	var r liveRef
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.Provenance.Version == "" || r.Provenance.CapturedAt == "" || r.TLS.JA4 == "" {
		t.Fatalf("%s: reference lacks provenance or fingerprint", name)
	}
	return r
}

// liveMeasure is one fresh connection's view through the echo service.
type liveMeasure struct {
	ja3Hash string
	ref     liveRef // same shape as a reference, filled from the response
}

func measureLive(t *testing.T, headers map[string]string, opts ...ClientOption) liveMeasure {
	t.Helper()
	var body []byte
	var status int
	var err error
	// The echo service resets connections now and then. Retry a few times,
	// then FAIL: an unreachable endpoint is never a pass.
	for attempt := range liveAttempts {
		bc, cerr := NewClient(append([]ClientOption{WithTimeout(20)}, opts...)...)
		if cerr != nil {
			t.Fatal(cerr)
		}
		body, _, status, err = bc.Do("GET", liveEndpoint, headers, nil)
		bc.Close()
		if err == nil && status == 200 {
			break
		}
		t.Logf("attempt %d/%d: status=%d err=%v", attempt+1, liveAttempts, status, err)
		time.Sleep(liveRetryPause)
	}
	if err != nil || status != 200 {
		t.Fatalf("endpoint unreachable or failing after %d attempts (this is a FAIL, not a skip): status=%d err=%v", liveAttempts, status, err)
	}
	var raw struct {
		TLS struct {
			JA3Hash       string `json:"ja3_hash"`
			JA4           string `json:"ja4"`
			JA4R          string `json:"ja4_r"`
			PeetprintHash string `json:"peetprint_hash"`
		} `json:"tls"`
		HTTP2 struct {
			Akamai string `json:"akamai_fingerprint"`
			Frames []struct {
				Type     string   `json:"frame_type"`
				Headers  []string `json:"headers"`
				Priority *struct {
					Weight    int `json:"weight"`
					Exclusive int `json:"exclusive"`
					DependsOn int `json:"depends_on"`
				} `json:"priority"`
			} `json:"sent_frames"`
		} `json:"http2"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("echo response is not the expected JSON: %v", err)
	}
	var m liveMeasure
	m.ja3Hash = raw.TLS.JA3Hash
	m.ref.TLS.JA4, m.ref.TLS.JA4R, m.ref.TLS.PeetprintHash = raw.TLS.JA4, raw.TLS.JA4R, raw.TLS.PeetprintHash
	m.ref.HTTP2.Akamai = raw.HTTP2.Akamai
	for _, f := range raw.HTTP2.Frames {
		if f.Type != "HEADERS" {
			continue
		}
		if f.Priority != nil {
			m.ref.HTTP2.Priority.Weight = f.Priority.Weight
			m.ref.HTTP2.Priority.Exclusive = f.Priority.Exclusive == 1
			m.ref.HTTP2.Priority.DependsOn = f.Priority.DependsOn
		}
		for _, line := range f.Headers {
			n, v, _ := strings.Cut(line, ": ")
			if strings.HasPrefix(n, ":") {
				continue
			}
			m.ref.HTTP2.RequestHeaderNames = append(m.ref.HTTP2.RequestHeaderNames, n)
			if n == "priority" {
				m.ref.HTTP2.PriorityHeader = v
			}
		}
		break
	}
	return m
}

// liveFetchHeaders is a fetch()-shaped header set for the identity of bp.
func liveFetchHeaders(bp BrowserProfile) map[string]string {
	h := map[string]string{
		"user-agent":      bp.UserAgent,
		"accept":          "*/*",
		"sec-fetch-site":  "same-origin",
		"sec-fetch-mode":  "cors",
		"sec-fetch-dest":  "empty",
		"accept-encoding": "gzip, deflate, br, zstd",
		"accept-language": "en-US,en;q=0.9",
	}
	for k, v := range ClientHintsHeaders(bp.UserAgent) {
		h[k] = v
	}
	return h
}

type liveRow struct {
	field, ours, want string
	asserted          bool
}

func (r liveRow) ok() bool { return r.ours == r.want }

func logTable(t *testing.T, title string, rows []liveRow) {
	var b strings.Builder
	fmt.Fprintf(&b, "\n== %s\n", title)
	for _, r := range rows {
		verdict := "MATCH"
		if !r.ok() {
			verdict = "DIFFER"
		}
		if !r.asserted {
			verdict += " (info, not asserted)"
		}
		fmt.Fprintf(&b, "  %-22s %-7s ours=%s\n  %-22s         ref =%s\n", r.field, verdict, r.ours, "", r.want)
	}
	t.Log(b.String())
	for _, r := range rows {
		if r.asserted && !r.ok() {
			t.Errorf("%s: %s differs\n  ours: %s\n  want: %s", title, r.field, r.ours, r.want)
		}
	}
}

func hdrNames(ns []string) string { return strings.Join(ns, ",") }

func TestWireLive(t *testing.T) {
	type tc struct {
		name     string
		opts     []ClientOption
		profile  BrowserProfile
		fetch    bool
		ref      string
		varyJA3  bool // true: JA3 must differ across connections; false: must be constant
		tlsExact bool
		// pinned: expected values recorded from this client, used when the
		// reference is not the right oracle (chrome_131).
		pinned *liveRef
	}
	bpFor := func(p TLSProfile) BrowserProfile {
		bp, ok := profileForTLS(p)
		if !ok {
			t.Fatalf("no BrowserProfile for %s", p)
		}
		return bp
	}
	def, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}
	defProfile := def.Identity().BrowserProfile
	def.Close()

	pin131 := liveRef{}
	// Measured through the echo service on 2026-11-04 with the ClientHello
	// pinned byte for byte by TestWire_Chrome131Frozen. These differ from
	// Chrome for Testing 131 (ja4 ..._d8a2da3f94cd) because of the old ALPS
	// codepoint; they are what existing consumers present.
	pin131.TLS.JA4 = "t13d1516h2_8daaf6152771_02713d6af862"
	pin131.TLS.PeetprintHash = "7466733991096b3f4e6c0e79b0083559"
	pin131.HTTP2.Akamai = "1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p"

	cases := []tc{
		{name: "chrome_156 navigation", opts: []ClientOption{WithProfile(ProfileChrome156)}, profile: bpFor(ProfileChrome156), ref: "google_chrome_156_nav.json", varyJA3: true, tlsExact: true},
		{name: "default navigation", opts: nil, profile: defProfile, ref: "google_chrome_156_nav.json", varyJA3: true, tlsExact: true},
		{name: "chrome_152 navigation", opts: []ClientOption{WithProfile(ProfileChrome152)}, profile: bpFor(ProfileChrome152), ref: "google_chrome_156_nav.json", varyJA3: true, tlsExact: true},
		{name: "chrome_156 fetch", opts: []ClientOption{WithProfile(ProfileChrome156), WithFetchMode()}, profile: bpFor(ProfileChrome156), fetch: true, ref: "google_chrome_156_fetch.json", varyJA3: true},
		{name: "chrome_131 navigation", opts: []ClientOption{WithProfile(ProfileChrome131)}, profile: bpFor(ProfileChrome131), ref: "cft_131_nav.json", varyJA3: false, pinned: &pin131},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ref := loadLiveRef(t, c.ref)
			headers := ChromeHeadersFor(c.profile)
			if c.fetch {
				headers = liveFetchHeaders(c.profile)
			}
			ja3 := map[string]bool{}
			var last liveMeasure
			for range liveSamples {
				last = measureLive(t, headers, append([]ClientOption{}, c.opts...)...)
				ja3[last.ja3Hash] = true
				if c.tlsExact && last.ref.TLS.JA4 != ref.TLS.JA4 {
					break // reported below with the offending value
				}
			}
			title := fmt.Sprintf("%s vs %s %s (%s, %s)", c.name, ref.Provenance.Browser, ref.Provenance.Version, ref.Provenance.Mode, ref.Provenance.CapturedAt)

			want := ref
			if c.pinned != nil {
				want = *c.pinned
			}
			rows := []liveRow{}
			add := func(field, ours, want string, asserted bool) {
				rows = append(rows, liveRow{field, ours, want, asserted})
			}
			add("ja4", last.ref.TLS.JA4, want.TLS.JA4, c.tlsExact || c.pinned != nil)
			if c.tlsExact {
				add("ja4_r", last.ref.TLS.JA4R, want.TLS.JA4R, true)
			}
			add("peetprint_hash", last.ref.TLS.PeetprintHash, want.TLS.PeetprintHash, c.tlsExact || c.pinned != nil)
			add("http2_akamai", last.ref.HTTP2.Akamai, want.HTTP2.Akamai, true)
			if c.pinned == nil {
				add("h2_priority", fmt.Sprintf("%+v", last.ref.HTTP2.Priority), fmt.Sprintf("%+v", want.HTTP2.Priority), true)
				wantPrio := ref.HTTP2.PriorityHeader
				add("priority_header", last.ref.HTTP2.PriorityHeader, wantPrio, c.fetch)
			}
			add("request_header_names", hdrNames(last.ref.HTTP2.RequestHeaderNames), hdrNames(ref.HTTP2.RequestHeaderNames), false)

			variance := fmt.Sprintf("%d distinct JA3 over %d connections", len(ja3), liveSamples)
			wantVar := "varies (>1)"
			okVar := len(ja3) > 1
			if !c.varyJA3 {
				wantVar, okVar = "constant (1)", len(ja3) == 1
			}
			t.Logf("  ja3 across connections: %s; want %s", variance, wantVar)
			if !okVar {
				t.Errorf("%s: %s, want %s", c.name, variance, wantVar)
			}
			logTable(t, title, rows)
		})
	}
}
