package stealth

import (
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
)

// jarBackends runs a sub-test on both backends: the option must behave the
// same on tls-client and on net/http.
var jarBackends = []struct {
	name string
	opts []ClientOption
}{
	{"tlsclient", nil},
	{"std", []ClientOption{WithStdHTTP()}},
}

func newJarClient(t *testing.T, extra []ClientOption, opts ...ClientOption) *BrowserClient {
	t.Helper()
	all := append([]ClientOption{WithoutSSRFGuard()}, extra...)
	all = append(all, opts...)
	c, err := NewClient(all...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func newJar(t *testing.T) *cookiejar.Jar {
	t.Helper()
	j, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

// cookieServer: /set?... sets the cookies given by repeated "c" query params
// (raw Set-Cookie lines, in order); anything else echoes the Cookie header.
func cookieServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			for _, line := range r.URL.Query()["c"] {
				w.Header().Add("Set-Cookie", line)
			}
			return
		}
		_, _ = w.Write([]byte(r.Header.Get("Cookie")))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setURL(base string, lines ...string) string {
	q := url.Values{"c": lines}
	return base + "/set?" + q.Encode()
}

func mustDo(t *testing.T, c *BrowserClient, u string) string {
	t.Helper()
	body, _, status, err := c.Do("GET", u, nil, nil)
	if err != nil || status != 200 {
		t.Fatalf("GET %s: status=%d err=%v", u, status, err)
	}
	return string(body)
}

func TestWithCookieJar_UsesCallerJar(t *testing.T) {
	for _, b := range jarBackends {
		t.Run(b.name, func(t *testing.T) {
			srv := cookieServer(t)
			u, _ := url.Parse(srv.URL)
			jar := newJar(t)
			jar.SetCookies(u, []*http.Cookie{{Name: "seed", Value: "1", Path: "/"}})
			c := newJarClient(t, b.opts, WithCookieJar(jar))

			// Seeded in the caller's jar -> sent by the client.
			if got := mustDo(t, c, srv.URL+"/echo"); got != "seed=1" {
				t.Fatalf("seeded cookie not sent: Cookie=%q", got)
			}
			// Set by a response -> readable from the caller's jar.
			mustDo(t, c, setURL(srv.URL, "got=2; Path=/"))
			var found bool
			for _, ck := range jar.Cookies(u) {
				found = found || (ck.Name == "got" && ck.Value == "2")
			}
			if !found {
				t.Fatalf("response cookie missing from caller jar: %v", jar.Cookies(u))
			}
			// Deleted through the jar -> no longer sent.
			jar.SetCookies(u, []*http.Cookie{{Name: "seed", Value: "", Path: "/", MaxAge: -1}})
			if got := mustDo(t, c, srv.URL+"/echo"); got != "got=2" {
				t.Fatalf("jar-deleted cookie still sent: Cookie=%q", got)
			}
			// GetCookieValue reads through the same jar.
			if v := c.GetCookieValue(srv.URL, "got"); v != "2" {
				t.Fatalf("GetCookieValue = %q, want 2", v)
			}
		})
	}
}

func TestWithCookieJar_ResponseDeletesCookie(t *testing.T) {
	for _, b := range jarBackends {
		t.Run(b.name, func(t *testing.T) {
			srv := cookieServer(t)
			c := newJarClient(t, b.opts, WithCookieJar(newJar(t)))
			mustDo(t, c, setURL(srv.URL, "k=v; Path=/"))
			if got := mustDo(t, c, srv.URL+"/echo"); got != "k=v" {
				t.Fatalf("Cookie=%q", got)
			}
			mustDo(t, c, setURL(srv.URL, "k=; Path=/; Max-Age=0"))
			if got := mustDo(t, c, srv.URL+"/echo"); got != "" {
				t.Fatalf("deleted cookie still sent: Cookie=%q", got)
			}
		})
	}
}

// Two clients (fetch + navigation) sharing one jar: a cookie set by a response
// to A is sent by B's next request.
func TestWithCookieJar_SharedAcrossClients(t *testing.T) {
	for _, b := range jarBackends {
		t.Run(b.name, func(t *testing.T) {
			srv := cookieServer(t)
			jar := newJar(t)
			a := newJarClient(t, b.opts, WithCookieJar(jar), WithFetchMode())
			nav := newJarClient(t, b.opts, WithCookieJar(jar))
			if b.name == "std" {
				a = newJarClient(t, b.opts, WithCookieJar(jar))
			}

			mustDo(t, a, setURL(srv.URL, "ct0=abc; Path=/"))
			if got := mustDo(t, nav, srv.URL+"/echo"); got != "ct0=abc" {
				t.Fatalf("client B did not see client A's cookie: Cookie=%q", got)
			}
			mustDo(t, nav, setURL(srv.URL, "from_b=1; Path=/"))
			if got := mustDo(t, a, srv.URL+"/echo"); got != "ct0=abc; from_b=1" {
				t.Fatalf("client A did not see client B's cookie: Cookie=%q", got)
			}
			// Without the option the jars are separate (control).
			c1, c2 := newJarClient(t, b.opts), newJarClient(t, b.opts)
			mustDo(t, c1, setURL(srv.URL, "solo=1; Path=/"))
			if got := mustDo(t, c2, srv.URL+"/echo"); got != "" {
				t.Fatalf("default jars leaked between clients: Cookie=%q", got)
			}
		})
	}
}

// Cookies set by ONE response are sent back in Set-Cookie order (Chrome and
// RFC 6265: longer path first, then earlier creation), on the wire.
func TestWithCookieJar_SetCookieOrderOnWire(t *testing.T) {
	for _, b := range jarBackends {
		t.Run(b.name, func(t *testing.T) {
			srv := cookieServer(t)
			c := newJarClient(t, b.opts, WithCookieJar(newJar(t)))
			mustDo(t, c, setURL(srv.URL, "a=1; Path=/", "b=2; Path=/", "c=3; Path=/", "deep=4; Path=/echo"))
			// RFC 6265 5.4: longer path first, then creation order.
			if got := mustDo(t, c, srv.URL+"/echo"); got != "deep=4; a=1; b=2; c=3" {
				t.Fatalf("Cookie order = %q, want %q", got, "deep=4; a=1; b=2; c=3")
			}
		})
	}
}

// Path scoping is the jar's: a /only cookie is not sent to /other.
func TestWithCookieJar_PathScopingOnWire(t *testing.T) {
	for _, b := range jarBackends {
		t.Run(b.name, func(t *testing.T) {
			srv := cookieServer(t)
			c := newJarClient(t, b.opts, WithCookieJar(newJar(t)))
			mustDo(t, c, setURL(srv.URL, "scoped=1; Path=/echo/only", "all=2; Path=/"))
			if got := mustDo(t, c, srv.URL+"/echo/other"); got != "all=2" {
				t.Fatalf("/echo/other Cookie=%q, want all=2", got)
			}
			if got := mustDo(t, c, srv.URL+"/echo/only"); got != "scoped=1; all=2" {
				t.Fatalf("/echo/only Cookie=%q, want scoped=1; all=2", got)
			}
		})
	}
}

// Domain scoping on the wire between two hostnames served by one listener:
// a parent-domain cookie set via api.x.localhost reaches www.x.localhost; a
// host-only cookie does not. Skipped where *.localhost does not resolve.
func TestWithCookieJar_DomainScopingOnWire(t *testing.T) {
	if addrs, err := net.LookupHost("api.x.localhost"); err != nil || len(addrs) == 0 {
		t.Skipf("api.x.localhost does not resolve here: %v", err)
	}
	for _, b := range jarBackends {
		t.Run(b.name, func(t *testing.T) {
			srv := cookieServer(t)
			_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
			api := "http://api.x.localhost:" + port
			www := "http://www.x.localhost:" + port
			c := newJarClient(t, b.opts, WithCookieJar(newJar(t)))
			mustDo(t, c, setURL(api, "shared=1; Domain=x.localhost; Path=/", "hostonly=2; Path=/"))
			if got := mustDo(t, c, www+"/echo"); got != "shared=1" {
				t.Fatalf("www.x.localhost Cookie=%q, want shared=1 only", got)
			}
			if got := mustDo(t, c, api+"/echo"); got != "hostonly=2; shared=1" && got != "shared=1; hostonly=2" {
				t.Fatalf("api.x.localhost Cookie=%q", got)
			}
		})
	}
}

// --- adapter unit tests (deterministic, no network) ---

func adapterFor(j http.CookieJar) fhttp.CookieJar { return adaptCookieJar(j) }

func fcookies(cs ...*fhttp.Cookie) []*fhttp.Cookie { return cs }

func names(cs []*fhttp.Cookie) string {
	var n []string
	for _, c := range cs {
		n = append(n, c.Name+"="+c.Value)
	}
	return strings.Join(n, ";")
}

func TestAdaptCookieJar_PathScoping(t *testing.T) {
	a := adapterFor(newJar(t))
	u, _ := url.Parse("https://api.example.test/a/x")
	a.SetCookies(u, fcookies(&fhttp.Cookie{Name: "p", Value: "1", Path: "/a"}, &fhttp.Cookie{Name: "q", Value: "2", Path: "/b"}))
	got := a.Cookies(u)
	if names(got) != "p=1" {
		t.Fatalf("cookies for /a/x = %q, want p=1", names(got))
	}
}

func TestAdaptCookieJar_DomainScoping(t *testing.T) {
	a := adapterFor(newJar(t))
	api, _ := url.Parse("https://api.example.test/")
	www, _ := url.Parse("https://www.example.test/")
	a.SetCookies(api, fcookies(
		&fhttp.Cookie{Name: "parent", Value: "1", Domain: "example.test", Path: "/"},
		&fhttp.Cookie{Name: "hostonly", Value: "2", Path: "/"},
	))
	if got := names(a.Cookies(www)); got != "parent=1" {
		t.Fatalf("sibling host sees %q, want parent=1 only", got)
	}
	if got := names(a.Cookies(api)); got != "hostonly=2;parent=1" && got != "parent=1;hostonly=2" {
		t.Fatalf("api host sees %q", got)
	}
}

func TestAdaptCookieJar_ExpiryAndDeletion(t *testing.T) {
	a := adapterFor(newJar(t))
	u, _ := url.Parse("https://example.test/")
	a.SetCookies(u, fcookies(
		&fhttp.Cookie{Name: "keep", Value: "1", Path: "/", Expires: time.Now().Add(time.Hour)},
		&fhttp.Cookie{Name: "gone", Value: "1", Path: "/"},
	))
	if got := names(a.Cookies(u)); got != "gone=1;keep=1" && got != "keep=1;gone=1" {
		t.Fatalf("setup: %q", got)
	}
	// Past Expires deletes.
	a.SetCookies(u, fcookies(&fhttp.Cookie{Name: "gone", Value: "", Path: "/", Expires: time.Now().Add(-time.Hour)}))
	if got := names(a.Cookies(u)); got != "keep=1" {
		t.Fatalf("Expires in the past did not delete: %q", got)
	}
	// MaxAge < 0 deletes.
	a.SetCookies(u, fcookies(&fhttp.Cookie{Name: "keep", Value: "", Path: "/", MaxAge: -1}))
	if got := names(a.Cookies(u)); got != "" {
		t.Fatalf("MaxAge<0 did not delete: %q", got)
	}
}

func TestAdaptCookieJar_SecureAndHTTPOnlyRoundTrip(t *testing.T) {
	a := adapterFor(newJar(t))
	https, _ := url.Parse("https://example.test/")
	plain, _ := url.Parse("http://example.test/")
	a.SetCookies(https, fcookies(&fhttp.Cookie{Name: "s", Value: "1", Path: "/", Secure: true}))
	if got := names(a.Cookies(plain)); got != "" {
		t.Fatalf("Secure cookie sent over http: %q", got)
	}
	if got := names(a.Cookies(https)); got != "s=1" {
		t.Fatalf("Secure cookie not sent over https: %q", got)
	}
}

func TestAdaptCookieJar_PreservesOrder(t *testing.T) {
	a := adapterFor(newJar(t))
	u, _ := url.Parse("https://example.test/")
	a.SetCookies(u, fcookies(
		&fhttp.Cookie{Name: "a", Value: "1", Path: "/"},
		&fhttp.Cookie{Name: "b", Value: "2", Path: "/"},
		&fhttp.Cookie{Name: "c", Value: "3", Path: "/"},
	))
	if got := names(a.Cookies(u)); got != "a=1;b=2;c=3" {
		t.Fatalf("order = %q, want a=1;b=2;c=3", got)
	}
}
