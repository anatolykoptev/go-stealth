package stealth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/bogdanfinn/fhttp/http2"
	"github.com/bogdanfinn/fhttp/http2/hpack"
)

// h2Capture is one request as a raw HTTP/2 server decoded it: the header
// fields in wire order (cookie crumbs kept apart, which net/http's server
// would merge) and the HEADERS priority.
type h2Capture struct {
	fields   []hpack.HeaderField
	streamID uint32
	prio     http2.PriorityParam
}

func (c h2Capture) values(name string) []string {
	var out []string
	for _, f := range c.fields {
		if f.Name == name {
			out = append(out, f.Value)
		}
	}
	return out
}

func (c h2Capture) names() []string {
	var out []string
	for _, f := range c.fields {
		out = append(out, f.Name)
	}
	return out
}

// startRawH2Server serves h2 over a self-signed cert on loopback, answers every
// request 200 with an empty body and sends each decoded request on the channel.
// Unlike net/http's server it exposes the wire: field-by-field headers and the
// HEADERS priority.
func startRawH2Server(t *testing.T) (addr string, got <-chan h2Capture) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   []string{"h2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ch := make(chan h2Capture, 16)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveRawH2(conn, ch)
		}
	}()
	return ln.Addr().String(), ch
}

func serveRawH2(conn net.Conn, out chan<- h2Capture) {
	defer conn.Close()
	preface := make([]byte, len(http2.ClientPreface))
	if _, err := io.ReadFull(conn, preface); err != nil {
		return
	}
	fr := http2.NewFramer(conn, conn)
	fr.ReadMetaHeaders = hpack.NewDecoder(65536, nil)
	_ = fr.WriteSettings()
	for {
		f, err := fr.ReadFrame()
		if err != nil {
			return
		}
		switch f := f.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				_ = fr.WriteSettingsAck()
			}
		case *http2.MetaHeadersFrame:
			select {
			case out <- h2Capture{fields: slices.Clone(f.Fields), streamID: f.StreamID, prio: f.Priority}:
			default:
			}
			var buf sliceWriter
			_ = hpack.NewEncoder(&buf).WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
			_ = fr.WriteHeaders(http2.HeadersFrameParam{StreamID: f.StreamID, BlockFragment: buf.b, EndHeaders: true, EndStream: true})
		}
	}
}

type sliceWriter struct{ b []byte }

func (w *sliceWriter) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }

func rawH2Get(t *testing.T, headers map[string]string, opts ...ClientOption) h2Capture {
	t.Helper()
	addr, got := startRawH2Server(t)
	bc, err := NewClient(append([]ClientOption{WithTimeout(10), WithInsecureSkipVerify(), WithoutSSRFGuard()}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	defer bc.Close()
	if _, _, status, err := bc.Do("GET", fmt.Sprintf("https://%s/", addr), headers, nil); err != nil || status != 200 {
		t.Fatalf("request failed: status=%d err=%v", status, err)
	}
	select {
	case c := <-got:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("server saw no request")
	}
	return h2Capture{}
}

// Real Chrome sends one `cookie` field per cookie pair on HTTP/2 (RFC 9113
// section 8.2.3; reference: Google Chrome 156 through the echo service shows
// "cookie: auth_token=..." and "cookie: ct0=..." as separate fields, in the
// slot of the cookie header). A client that sends "a=1; b=2" as one field is
// distinguishable from it by its HPACK shape alone, and nothing errors when a
// dependency bump drops the split, so this pins the wire.
func TestH2_CookieCrumbs(t *testing.T) {
	cases := []struct {
		name string
		opts []ClientOption
	}{
		{"chrome_156", []ClientOption{WithProfile(ProfileChrome156)}},
		{"chrome_156 fetch", []ClientOption{WithProfile(ProfileChrome156), WithFetchMode()}},
		{"chrome_152", []ClientOption{WithProfile(ProfileChrome152)}},
		{"chrome_131", []ClientOption{WithProfile(ProfileChrome131)}},
	}
	for _, tc := range cases {
		{
			opts := tc.opts
			t.Run(tc.name, func(t *testing.T) {
				c := rawH2Get(t, map[string]string{
					"User-Agent": "x", "Accept": "*/*", "Accept-Language": "en-US,en;q=0.9",
					"Cookie": "auth_token=aaa; ct0=bbb; lang=en",
				}, opts...)
				got := c.values("cookie")
				want := []string{"auth_token=aaa", "ct0=bbb", "lang=en"}
				if !slices.Equal(got, want) {
					t.Fatalf("cookie fields = %q, want one field per pair %q", got, want)
				}
				// The crumbs stay together, in the slot of the cookie header.
				names := c.names()
				first := slices.Index(names, "cookie")
				if first < 0 || !slices.Equal(names[first:first+3], []string{"cookie", "cookie", "cookie"}) {
					t.Fatalf("cookie crumbs are not contiguous: %v", names)
				}
			})
		}
	}
}
