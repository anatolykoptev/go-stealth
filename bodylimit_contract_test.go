package stealth

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Only the codings the package decodes get the framing allowance; a coding it
// leaves alone (br, unknown names) is read as raw bytes under exactly limit.
func TestWireLimit_AllowanceOnlyForDecodedCodings(t *testing.T) {
	for _, enc := range []string{"", "identity", "br", "BR", "compress", "x-unknown"} {
		if got := wireLimit(testLimit, enc); got != testLimit {
			t.Errorf("wireLimit(%d, %q) = %d, want exactly the limit", testLimit, enc, got)
		}
	}
	for _, enc := range []string{"gzip", "GZIP", "deflate", "zstd", " gzip "} {
		if got := wireLimit(testLimit, enc); got <= testLimit {
			t.Errorf("wireLimit(%d, %q) = %d, want a framing allowance past the limit", testLimit, enc, got)
		}
	}
}

func TestMaxBody_UndecodedCodingObeysLimit(t *testing.T) {
	for _, enc := range []string{"br", "x-unknown"} {
		srv := serveBytes(t, map[string]string{"Content-Encoding": enc}, bytes.Repeat([]byte("u"), testLimit+10))
		for name := range backendOpts {
			if name == "tls" && enc == "br" {
				continue // tls-client decodes br in its transport; the body here is not a brotli stream
			}
			t.Run(name+"/"+enc, func(t *testing.T) {
				if _, _, _, err := newLimited(t, name, testLimit).Do("GET", srv.URL, nil, nil); !errors.Is(err, ErrBodyTooLarge) {
					t.Fatalf("err = %v, want ErrBodyTooLarge", err)
				}
			})
		}
	}
}

func TestReadBodyLimit(t *testing.T) {
	src := &countingReader{r: strings.NewReader(strings.Repeat("x", 10_000))}
	if _, err := ReadBodyLimit(src, 100); !errors.Is(err, ErrBodyTooLarge) || src.n != 101 {
		t.Fatalf("over limit: err = %v after %d bytes, want ErrBodyTooLarge after exactly 101", err, src.n)
	}
	if got, err := ReadBodyLimit(strings.NewReader(strings.Repeat("x", 100)), 100); err != nil || len(got) != 100 {
		t.Fatalf("at limit: %d bytes, err %v", len(got), err)
	}
	if got, err := ReadBodyLimit(strings.NewReader(strings.Repeat("x", 500)), 0); err != nil || len(got) != 500 {
		t.Fatalf("limit 0 must be unbounded: %d bytes, err %v", len(got), err)
	}
}

// A custom backend that follows the contract with the exported helper stops
// reading at the limit, and the client reports it.
func TestMaxBody_CustomBackendUsingReadBodyLimit(t *testing.T) {
	src := &countingReader{r: strings.NewReader(strings.Repeat("c", 10*testLimit))}
	factory := func(cfg BackendConfig) (HTTPDoer, error) {
		return &readerDoer{r: src, limit: cfg.MaxBodyBytes}, nil
	}
	bc, err := NewClient(WithoutSSRFGuard(), WithBackend(factory), WithMaxBodyBytes(testLimit))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := bc.Do("GET", "https://example.com", nil, nil); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
	if src.n != testLimit+1 {
		t.Fatalf("backend pulled %d bytes, want exactly limit+1 = %d", src.n, testLimit+1)
	}
}

type readerDoer struct {
	r     io.Reader
	limit int64
}

func (d *readerDoer) Do(*Request) (*Response, error) {
	b, err := ReadBodyLimit(d.r, d.limit)
	if err != nil {
		return &Response{StatusCode: http.StatusOK}, err
	}
	return &Response{StatusCode: http.StatusOK, Body: b, Headers: map[string]string{}}, nil
}
func (d *readerDoer) SetProxy(string) error             { return nil }
func (d *readerDoer) GetCookieValue(_, _ string) string { return "" }
