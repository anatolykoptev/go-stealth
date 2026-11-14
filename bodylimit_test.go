package stealth

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

const (
	testLimit    = 1 << 20  // 1 MiB limit used by most tests
	testBombSize = 50 << 20 // decoded size of the compression bombs
	// socketSlack is how far past the limit a server can get before the
	// client's close reaches it: kernel socket buffers on both ends.
	socketSlack = 24 << 20
)

// --- helpers ---------------------------------------------------------------

var bombs = sync.OnceValue(func() map[string][]byte {
	zeros := make([]byte, testBombSize)

	var gz bytes.Buffer
	gw := gzip.NewWriter(&gz)
	_, _ = gw.Write(zeros)
	_ = gw.Close()

	var df bytes.Buffer
	zw := zlib.NewWriter(&df)
	_, _ = zw.Write(zeros)
	_ = zw.Close()

	enc, _ := zstd.NewWriter(nil)
	zs := enc.EncodeAll(zeros, nil)
	_ = enc.Close()

	return map[string][]byte{"gzip": gz.Bytes(), "deflate": df.Bytes(), "zstd": zs}
})

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func gzipped(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

// backendOpts are the two real backends every wire test runs against.
var backendOpts = map[string][]ClientOption{
	"std": {WithStdHTTP()},
	"tls": nil,
}

func newLimited(t *testing.T, backend string, limit int64, extra ...ClientOption) *BrowserClient {
	t.Helper()
	opts := append([]ClientOption{WithoutSSRFGuard(), WithTimeout(30)}, backendOpts[backend]...)
	if limit > 0 {
		opts = append(opts, WithMaxBodyBytes(limit))
	}
	bc, err := NewClient(append(opts, extra...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bc.Close() })
	return bc
}

// countingReader counts the bytes pulled through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// stream is a chunked body of total bytes that records how much the server
// managed to write and signals when its handler has returned (a closed
// connection makes the next write fail).
type stream struct {
	written atomic.Int64
	done    chan struct{}
}

func newStream() *stream { return &stream{done: make(chan struct{})} }

func (s *stream) handler(total int64, header map[string]string, prefix []byte) http.HandlerFunc {
	chunk := bytes.Repeat([]byte("a"), 64<<10)
	return func(w http.ResponseWriter, _ *http.Request) {
		defer close(s.done)
		for k, v := range header {
			w.Header().Set(k, v)
		}
		fl, _ := w.(http.Flusher)
		if len(prefix) > 0 {
			n, _ := w.Write(prefix)
			s.written.Add(int64(n))
		}
		for s.written.Load() < total {
			n, err := w.Write(chunk)
			s.written.Add(int64(n))
			if err != nil {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
	}
}

func (s *stream) waitClosed(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
		t.Fatal("server handler still writing 10s after Do returned: the connection was drained, not closed")
	}
}

func serveBytes(t *testing.T, header map[string]string, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for k, v := range header {
			w.Header().Set(k, v)
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// --- readAllLimit ----------------------------------------------------------

func TestReadAllLimit_StopsAtLimitPlusOne(t *testing.T) {
	src := &countingReader{r: strings.NewReader(strings.Repeat("x", 10_000))}
	_, err := readAllLimit(src, 100)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
	if src.n != 101 {
		t.Fatalf("read %d bytes from the source, want exactly limit+1 = 101", src.n)
	}
}

func TestReadAllLimit_ExactlyAtLimit(t *testing.T) {
	got, err := readAllLimit(strings.NewReader(strings.Repeat("x", 100)), 100)
	if err != nil || len(got) != 100 {
		t.Fatalf("got %d bytes, err %v; want 100 bytes, nil", len(got), err)
	}
}

func TestReadAllLimit_ZeroIsUnbounded(t *testing.T) {
	got, err := readAllLimit(strings.NewReader(strings.Repeat("x", 5000)), 0)
	if err != nil || len(got) != 5000 {
		t.Fatalf("got %d bytes, err %v; want 5000 bytes, nil", len(got), err)
	}
}

// --- decoding --------------------------------------------------------------

func TestDecodeBody_BombRejectedAndBounded(t *testing.T) {
	for enc, bomb := range bombs() {
		t.Run(enc, func(t *testing.T) {
			_, err := decodeBody(bomb, enc, testLimit)
			if !errors.Is(err, ErrBodyTooLarge) {
				t.Fatalf("decodeBody(%s bomb, limit %d) err = %v, want ErrBodyTooLarge", enc, testLimit, err)
			}
		})
	}
}

// A bomb with its Content-Encoding header stripped is found by magic bytes.
func TestDecodeBody_HeaderlessBombRejected(t *testing.T) {
	for _, enc := range []string{"gzip", "zstd"} {
		if _, err := decodeBody(bombs()[enc], "", testLimit); !errors.Is(err, ErrBodyTooLarge) {
			t.Fatalf("headerless %s bomb: err = %v, want ErrBodyTooLarge", enc, err)
		}
	}
}

// The decoder must stop early, not decode 50 MB and then compare: under a
// 1 MiB limit each coding allocates a few MiB at most, and 50 MiB or more
// without it.
func TestDecodeBody_BombAllocationBounded(t *testing.T) {
	for _, enc := range []string{"gzip", "deflate", "zstd"} {
		t.Run(enc, func(t *testing.T) {
			bomb := bombs()[enc]
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			_, err := decodeBody(bomb, enc, testLimit)
			runtime.ReadMemStats(&after)
			if !errors.Is(err, ErrBodyTooLarge) {
				t.Fatalf("err = %v, want ErrBodyTooLarge", err)
			}
			if got := after.TotalAlloc - before.TotalAlloc; got > 16<<20 {
				t.Fatalf("decoding a %d-byte bomb under a %d-byte limit allocated %d bytes", testBombSize, testLimit, got)
			}
		})
	}
}

func TestDecodeBody_AtLimitSucceeds(t *testing.T) {
	plain := bytes.Repeat([]byte("z"), testLimit)
	var df bytes.Buffer
	zw := zlib.NewWriter(&df)
	_, _ = zw.Write(plain)
	_ = zw.Close()
	enc, _ := zstd.NewWriter(nil)
	defer enc.Close()

	cases := map[string][]byte{
		"":        plain,
		"gzip":    gzipped(plain),
		"deflate": df.Bytes(),
		"zstd":    enc.EncodeAll(plain, nil),
	}
	for name, packed := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := decodeBody(packed, name, testLimit)
			if err != nil || !bytes.Equal(got, plain) {
				t.Fatalf("at-limit body: len %d, err %v; want %d bytes, nil", len(got), err, len(plain))
			}
		})
	}
}

// Undecodable input falls back to the raw bytes; those obey the limit too.
func TestDecodeBody_RawFallbackObeysLimit(t *testing.T) {
	junk := bytes.Repeat([]byte("not gzip "), testLimit/9+10)[:testLimit+10]
	if _, err := decodeBody(junk, "gzip", testLimit); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("raw fallback of %d bytes under a %d limit: err = %v, want ErrBodyTooLarge", len(junk), testLimit, err)
	}
}

// --- through the backends --------------------------------------------------

func TestMaxBody_PlainOversizeStopsAndClosesConnection(t *testing.T) {
	for name := range backendOpts {
		t.Run(name, func(t *testing.T) {
			s := newStream()
			srv := httptest.NewServer(s.handler(256<<20, nil, nil))
			defer srv.Close()

			_, _, status, err := newLimited(t, name, testLimit).Do("GET", srv.URL, nil, nil)
			if !errors.Is(err, ErrBodyTooLarge) {
				t.Fatalf("err = %v, want ErrBodyTooLarge", err)
			}
			if status != http.StatusOK {
				t.Errorf("status = %d, want 200 carried with the error", status)
			}
			s.waitClosed(t)
			if w := s.written.Load(); w > testLimit+socketSlack {
				t.Fatalf("server wrote %d bytes before the close reached it, want <= %d", w, testLimit+socketSlack)
			}
		})
	}
}

func TestMaxBody_CompressedBombRejected(t *testing.T) {
	for name := range backendOpts {
		for enc, bomb := range bombs() {
			t.Run(name+"/"+enc, func(t *testing.T) {
				srv := serveBytes(t, map[string]string{"Content-Encoding": enc}, bomb)
				body, _, _, err := newLimited(t, name, testLimit).Do("GET", srv.URL, nil, nil)
				if !errors.Is(err, ErrBodyTooLarge) {
					t.Fatalf("%d-byte %s bomb: err = %v (body %d bytes), want ErrBodyTooLarge", len(bomb), enc, err, len(body))
				}
				if len(body) != 0 {
					t.Errorf("body of %d bytes returned alongside the error", len(body))
				}
			})
		}
	}
}

func TestMaxBody_AtLimitSucceeds(t *testing.T) {
	plain := bytes.Repeat([]byte("p"), testLimit)
	// Incompressible data: the gzip stream is larger than the body it holds.
	noise := randomBytes(testLimit)
	var df bytes.Buffer
	zw := zlib.NewWriter(&df)
	_, _ = zw.Write(noise)
	_ = zw.Close()
	zenc, _ := zstd.NewWriter(nil)
	defer zenc.Close()
	cases := map[string]struct {
		header map[string]string
		wire   []byte
		want   []byte
	}{
		"plain": {nil, plain, plain},
		"gzip":  {map[string]string{"Content-Encoding": "gzip"}, gzipped(plain), plain},
		// The wire form of incompressible data is longer than the body.
		"gzip-noise":      {map[string]string{"Content-Encoding": "gzip"}, gzipped(noise), noise},
		"deflate-noise":   {map[string]string{"Content-Encoding": "deflate"}, df.Bytes(), noise},
		"zstd-noise":      {map[string]string{"Content-Encoding": "zstd"}, zenc.EncodeAll(noise, nil), noise},
		"plain-one-under": {nil, plain[:testLimit-1], plain[:testLimit-1]},
	}
	for name := range backendOpts {
		for cname, c := range cases {
			t.Run(name+"/"+cname, func(t *testing.T) {
				srv := serveBytes(t, c.header, c.wire)
				body, _, _, err := newLimited(t, name, testLimit).Do("GET", srv.URL, nil, nil)
				if err != nil {
					t.Fatalf("body of %d bytes (wire %d) under limit %d: %v", len(c.want), len(c.wire), testLimit, err)
				}
				if !bytes.Equal(body, c.want) {
					t.Fatalf("got %d bytes, want %d", len(body), len(c.want))
				}
			})
		}
	}
}

func TestMaxBody_OneOverLimitRejected(t *testing.T) {
	srv := serveBytes(t, nil, bytes.Repeat([]byte("p"), testLimit+1))
	for name := range backendOpts {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := newLimited(t, name, testLimit).Do("GET", srv.URL, nil, nil); !errors.Is(err, ErrBodyTooLarge) {
				t.Fatalf("limit+1 body: err = %v, want ErrBodyTooLarge", err)
			}
		})
	}
}

func TestMaxBody_DefaultClientReadsLargeBody(t *testing.T) {
	big := bytes.Repeat([]byte("L"), 8*testLimit)
	srv := serveBytes(t, nil, big)
	bomb := serveBytes(t, map[string]string{"Content-Encoding": "gzip"}, bombs()["gzip"])
	for name := range backendOpts {
		t.Run(name, func(t *testing.T) {
			bc := newLimited(t, name, 0)
			body, _, _, err := bc.Do("GET", srv.URL, nil, nil)
			if err != nil || len(body) != len(big) {
				t.Fatalf("default client: %d bytes, err %v; want %d bytes, nil", len(body), err, len(big))
			}
			body, _, _, err = bc.Do("GET", bomb.URL, nil, nil)
			if err != nil || len(body) != testBombSize {
				t.Fatalf("default client, gzip: %d bytes, err %v; want %d bytes, nil", len(body), err, testBombSize)
			}
		})
	}
}

// Every way into the client shares the limit.
func TestMaxBody_AllEntryPoints(t *testing.T) {
	srv := serveBytes(t, nil, bytes.Repeat([]byte("p"), testLimit+1))
	bc := newLimited(t, "std", testLimit)

	entries := map[string]func() error{
		"Do": func() error {
			_, _, _, err := bc.Do("GET", srv.URL, nil, nil)
			return err
		},
		"DoCtx": func() error {
			_, _, _, err := bc.DoCtx(context.Background(), "GET", srv.URL, nil, nil)
			return err
		},
		"DoWithHeaderOrder": func() error {
			_, _, _, err := bc.DoWithHeaderOrder("GET", srv.URL, nil, nil, []string{"user-agent"})
			return err
		},
		"DoWithHeaderOrderCtx": func() error {
			_, _, _, err := bc.DoWithHeaderOrderCtx(context.Background(), "GET", srv.URL, nil, nil, []string{"user-agent"})
			return err
		},
		"RoundTrip": func() error {
			req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
			_, err := bc.StdClient().Do(req)
			return err
		},
	}
	for name, call := range entries {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrBodyTooLarge) {
				t.Fatalf("err = %v, want ErrBodyTooLarge", err)
			}
		})
	}
}

// The limit applies to the body at the end of a redirect chain.
func TestMaxBody_AfterRedirect(t *testing.T) {
	srv := serveBytes(t, nil, bytes.Repeat([]byte("p"), testLimit+1))
	hop := httptest.NewServer(http.RedirectHandler(srv.URL, http.StatusFound))
	defer hop.Close()
	for name := range backendOpts {
		t.Run(name, func(t *testing.T) {
			bc := newLimited(t, name, testLimit, WithFollowRedirects(), WithRedirectGuard(func(*http.Request, []*http.Request) error { return nil }))
			if _, _, _, err := bc.Do("GET", hop.URL, nil, nil); !errors.Is(err, ErrBodyTooLarge) {
				t.Fatalf("err = %v, want ErrBodyTooLarge", err)
			}
		})
	}
}

// --- client-level ----------------------------------------------------------

func TestWithMaxBodyBytes_RejectsNonPositive(t *testing.T) {
	for _, n := range []int64{0, -1} {
		if _, err := NewClient(WithoutSSRFGuard(), WithStdHTTP(), WithMaxBodyBytes(n)); err == nil {
			t.Fatalf("WithMaxBodyBytes(%d): NewClient returned no error; a zero limit must not mean unbounded", n)
		}
	}
}

// bodyDoer is a caller-supplied backend that ignores BackendConfig.MaxBodyBytes.
type bodyDoer struct {
	body  []byte
	err   error
	calls atomic.Int64
}

func (d *bodyDoer) Do(*Request) (*Response, error) {
	d.calls.Add(1)
	if d.err != nil {
		return &Response{StatusCode: http.StatusOK}, d.err
	}
	return &Response{StatusCode: http.StatusOK, Body: d.body, Headers: map[string]string{}}, nil
}
func (d *bodyDoer) SetProxy(string) error             { return nil }
func (d *bodyDoer) GetCookieValue(_, _ string) string { return "" }

func TestMaxBody_CustomBackendHeldToLimit(t *testing.T) {
	d := &bodyDoer{body: bytes.Repeat([]byte("c"), testLimit+1)}
	bc, err := NewClient(WithoutSSRFGuard(), WithBackend(func(BackendConfig) (HTTPDoer, error) { return d, nil }), WithMaxBodyBytes(testLimit))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := bc.Do("GET", "https://example.com", nil, nil); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
	d.body = d.body[:testLimit]
	if _, _, _, err := bc.Do("GET", "https://example.com", nil, nil); err != nil {
		t.Fatalf("body at the limit from a custom backend: %v", err)
	}
}

func TestMaxBody_NotRetriedOnAnotherProxy(t *testing.T) {
	d := &bodyDoer{err: bodyTooLarge(testLimit)}
	pool := &mockPool{proxies: []string{"p1", "p2", "p3"}}
	bc, err := NewClient(WithoutSSRFGuard(), WithBackend(func(BackendConfig) (HTTPDoer, error) { return d, nil }),
		WithProxyPool(pool), WithRetryOnBlock(2), WithMaxBodyBytes(testLimit))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := bc.Do("GET", "https://example.com", nil, nil); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
	if got := d.calls.Load(); got != 1 {
		t.Fatalf("backend called %d times, want 1: an oversized body must not be pulled again through another proxy", got)
	}
}

// --- ox-browser fallback ---------------------------------------------------

// cfChallengeBackend answers every request with a Cloudflare challenge page.
func cfChallengeBackend() BackendFactory {
	return func(BackendConfig) (HTTPDoer, error) {
		return &cfDoer{}, nil
	}
}

type cfDoer struct{}

func (*cfDoer) Do(*Request) (*Response, error) {
	return &Response{
		Body:       []byte(`<script src="/cdn-cgi/challenge-platform/x.js"></script>`),
		StatusCode: http.StatusServiceUnavailable,
		Headers:    map[string]string{"server": "cloudflare"},
	}, nil
}
func (*cfDoer) SetProxy(string) error             { return nil }
func (*cfDoer) GetCookieValue(_, _ string) string { return "" }

// oxReply serves /fetch-smart with a JSON reply holding a body of n bytes,
// written in chunks, and records how much of it left the server.
func oxReply(t *testing.T, n int) (*httptest.Server, *stream) {
	t.Helper()
	s := newStream()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fetch-smart" {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "cookies": map[string]string{"cf_clearance": "x"}})
			return
		}
		s.handler(int64(n), nil, []byte(`{"status":200,"method":"solved","cf_detected":true,"body":"`))(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, s
}

func newOxClient(t *testing.T, oxURL string, limit int64) *BrowserClient {
	t.Helper()
	bc, err := NewClient(WithoutSSRFGuard(), WithBackend(cfChallengeBackend()), WithOxBrowser(oxURL), WithMaxBodyBytes(limit))
	if err != nil {
		t.Fatal(err)
	}
	return bc
}

// A page body inside the ox-browser reply is bigger than the limit but the
// reply is complete and small: the finished body is checked.
func TestMaxBody_OxBrowserReplacedBodyHeldToLimit(t *testing.T) {
	big, err := json.Marshal(map[string]any{"status": 200, "method": "solved", "cf_detected": true,
		"body": strings.Repeat("o", 2048)})
	if err != nil {
		t.Fatal(err)
	}
	srv := serveBytes(t, nil, big)
	bc := newOxClient(t, srv.URL, 1024)
	_, _, _, err = bc.Do("GET", "https://example.com/page", nil, nil)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge, not the challenge error", err)
	}
}

// The reply itself is read under a bound: an endless reply is cut off.
func TestMaxBody_OxBrowserReplyReadBounded(t *testing.T) {
	const limit = 1024
	srv, s := oxReply(t, 64<<20)
	bc := newOxClient(t, srv.URL, limit)
	_, _, _, err := bc.Do("GET", "https://example.com/page", nil, nil)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
	s.waitClosed(t)
	if w, max := s.written.Load(), oxEnvelopeLimit(limit)+socketSlack; w > max {
		t.Fatalf("ox-browser wrote %d bytes of a 64 MiB reply, want <= %d", w, max)
	}
}

// Unrelated to the limit: a normal reply still goes through.
func TestMaxBody_OxBrowserWithinLimit(t *testing.T) {
	srv := serveBytes(t, nil, []byte(`{"status":200,"method":"solved","cf_detected":true,"body":"<html>ok</html>"}`))
	body, _, status, err := newOxClient(t, srv.URL, 1024).Do("GET", "https://example.com/page", nil, nil)
	if err != nil || status != 200 || string(body) != "<html>ok</html>" {
		t.Fatalf("got %q status %d err %v", body, status, err)
	}
}
