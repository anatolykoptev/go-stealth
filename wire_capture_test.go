package stealth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"
)

// This file is the test-side wire oracle: a local TLS+HTTP/2 listener that
// records what a client actually puts on the wire — the ClientHello (cipher
// suites, extension order, ALPS codepoint) and the first HTTP/2 frames
// (SETTINGS, WINDOW_UPDATE, the HEADERS priority). Asserting on the wire,
// not on the options passed to tls-client, is what makes these tests fail
// when a profile setting is silently dropped.

const (
	greaseNormalised = 0x0a0a
	h2PrefaceLen     = 24
	h2FrameHeaderLen = 9
	h2TypeHeaders    = 0x1
	h2TypeSettings   = 0x4
	h2TypeWindowUpd  = 0x8
	h2FlagPadded     = 0x8
	h2FlagPriority   = 0x20
)

type h2Priority struct {
	Exclusive bool
	DependsOn uint32
	// Wire is the on-wire weight byte; the effective HTTP/2 weight (the number
	// echo services print) is Wire+1.
	Wire uint8
}

// Weight is the effective HTTP/2 weight (RFC 9113 section 5.3.2: wire + 1).
func (p h2Priority) Weight() int { return int(p.Wire) + 1 }

type wireCapture struct {
	Ciphers      []uint16 // GREASE normalised to 0x0a0a
	Extensions   []uint16 // GREASE normalised to 0x0a0a, wire order
	ExtData      map[uint16][]byte
	Settings     [][2]uint32
	WindowUpdate uint32
	Priority     *h2Priority
}

func isGrease(v uint16) bool {
	return v&0x0f0f == 0x0a0a && v>>8 == v&0xff
}

func norm(v uint16) uint16 {
	if isGrease(v) {
		return greaseNormalised
	}
	return v
}

// recordingConn keeps every byte the server side reads, so the ClientHello can
// be parsed after the handshake completes.
type recordingConn struct {
	net.Conn
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *recordingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.mu.Lock()
	c.buf.Write(p[:n])
	c.mu.Unlock()
	return n, err
}

func (c *recordingConn) recorded() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.buf.Bytes()...)
}

func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// parseClientHello extracts cipher suites and extension IDs (wire order) from
// the first TLS record in raw.
func parseClientHello(raw []byte) (ciphers, exts []uint16, data map[uint16][]byte, err error) {
	data = map[uint16][]byte{}
	bad := fmt.Errorf("malformed ClientHello (%d bytes)", len(raw))
	if len(raw) < 5+4+2+32+1 || raw[0] != 0x16 || raw[5] != 0x01 {
		return nil, nil, nil, bad
	}
	p := raw[5+4:]
	p = p[2+32:] // version, random
	if len(p) < 1 || len(p) < 1+int(p[0]) {
		return nil, nil, nil, bad
	}
	p = p[1+int(p[0]):] // session id
	if len(p) < 2 {
		return nil, nil, nil, bad
	}
	cl := int(binary.BigEndian.Uint16(p))
	if len(p) < 2+cl {
		return nil, nil, nil, bad
	}
	for i := 0; i+1 < cl; i += 2 {
		ciphers = append(ciphers, norm(binary.BigEndian.Uint16(p[2+i:])))
	}
	p = p[2+cl:]
	if len(p) < 1 || len(p) < 1+int(p[0]) {
		return nil, nil, nil, bad
	}
	p = p[1+int(p[0]):] // compression
	if len(p) < 2 {
		return nil, nil, nil, bad
	}
	el := int(binary.BigEndian.Uint16(p))
	p = p[2:]
	if len(p) < el {
		return nil, nil, nil, bad
	}
	p = p[:el]
	for len(p) >= 4 {
		id := binary.BigEndian.Uint16(p)
		l := int(binary.BigEndian.Uint16(p[2:]))
		if len(p) < 4+l {
			return nil, nil, nil, bad
		}
		exts = append(exts, norm(id))
		data[norm(id)] = append([]byte(nil), p[4:4+l]...)
		p = p[4+l:]
	}
	return ciphers, exts, data, nil
}

// readH2 reads the client preface and frames up to and including the first
// HEADERS frame.
func readH2(c io.Reader, w *wireCapture) error {
	if _, err := io.ReadFull(c, make([]byte, h2PrefaceLen)); err != nil {
		return fmt.Errorf("preface: %w", err)
	}
	for {
		hdr := make([]byte, h2FrameHeaderLen)
		if _, err := io.ReadFull(c, hdr); err != nil {
			return fmt.Errorf("frame header: %w", err)
		}
		length := int(hdr[0])<<16 | int(hdr[1])<<8 | int(hdr[2])
		typ, flags := hdr[3], hdr[4]
		payload := make([]byte, length)
		if _, err := io.ReadFull(c, payload); err != nil {
			return fmt.Errorf("frame payload: %w", err)
		}
		switch typ {
		case h2TypeSettings:
			for i := 0; i+6 <= len(payload); i += 6 {
				w.Settings = append(w.Settings, [2]uint32{
					uint32(binary.BigEndian.Uint16(payload[i:])),
					binary.BigEndian.Uint32(payload[i+2:]),
				})
			}
		case h2TypeWindowUpd:
			w.WindowUpdate = binary.BigEndian.Uint32(payload) & 0x7fffffff
		case h2TypeHeaders:
			if flags&h2FlagPriority != 0 {
				off := 0
				if flags&h2FlagPadded != 0 {
					off = 1
				}
				dep := binary.BigEndian.Uint32(payload[off:])
				w.Priority = &h2Priority{Exclusive: dep>>31 == 1, DependsOn: dep & 0x7fffffff, Wire: payload[off+4]}
			}
			return nil
		}
	}
}

// captureWire starts a local TLS+h2 listener, drives one request from a
// client built with opts, and returns what the client sent. Each call is one
// fresh connection. The client trusts the listener's throwaway certificate.
func captureWire(t *testing.T, opts ...ClientOption) wireCapture {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	cfg := &tls.Config{Certificates: []tls.Certificate{selfSignedCert(t)}, NextProtos: []string{"h2"}}

	type result struct {
		w   wireCapture
		err error
	}
	done := make(chan result, 1)
	go func() {
		raw, err := ln.Accept()
		if err != nil {
			done <- result{err: err}
			return
		}
		defer func() { _ = raw.Close() }()
		_ = raw.SetDeadline(time.Now().Add(10 * time.Second))
		rec := &recordingConn{Conn: raw}
		conn := tls.Server(rec, cfg)
		if err := conn.Handshake(); err != nil {
			done <- result{err: fmt.Errorf("handshake: %w", err)}
			return
		}
		var w wireCapture
		w.Ciphers, w.Extensions, w.ExtData, err = parseClientHello(rec.recorded())
		if err != nil {
			done <- result{err: err}
			return
		}
		// An empty SETTINGS frame so the client does not wait on the server.
		_, _ = conn.Write([]byte{0, 0, 0, h2TypeSettings, 0, 0, 0, 0, 0})
		done <- result{w, readH2(conn, &w)}
	}()

	all := append([]ClientOption{WithInsecureSkipVerify(), WithoutSSRFGuard(), WithTimeout(5)}, opts...)
	bc, err := NewClient(all...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bc.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port
	// The server closes after the first HEADERS frame, so Do fails; only the
	// captured bytes matter.
	_, _, _, _ = bc.Do("GET", fmt.Sprintf("https://localhost:%d/", port), nil, nil)

	r := <-done
	if r.err != nil {
		t.Fatalf("wire capture: %v", r.err)
	}
	return r.w
}

func sameU16(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// payloadHex renders the payload of extension id as hex, with GREASE values
// inside the u16 lists of supported_groups (0x0a) and supported_versions
// (0x2b) normalised, so the string is the same on every connection. It covers
// the extensions whose payload is deterministic (key_share, ECH and the trust
// anchors carry per-connection randomness and are not included).
func payloadHex(w wireCapture, id uint16) string {
	d, ok := w.ExtData[id]
	if !ok {
		return "absent"
	}
	d = append([]byte(nil), d...)
	switch id {
	case 0x0a: // u16 length, then u16 groups
		for i := 2; i+1 < len(d); i += 2 {
			binary.BigEndian.PutUint16(d[i:], norm(binary.BigEndian.Uint16(d[i:])))
		}
	case 0x2b: // u8 length, then u16 versions
		for i := 1; i+1 < len(d); i += 2 {
			binary.BigEndian.PutUint16(d[i:], norm(binary.BigEndian.Uint16(d[i:])))
		}
	}
	return fmt.Sprintf("%x", d)
}
