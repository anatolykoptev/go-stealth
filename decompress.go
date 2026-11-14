package stealth

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

const (
	// zstdMaxWindow is the largest window a zstd content coding may use
	// (RFC 9659 section 3); a bigger one is not a valid HTTP body.
	zstdMaxWindow = 8 << 20
	// zstdMaxDecoded bounds one decoded body. A few kilobytes of zstd can
	// describe gigabytes, and the decoder allocates what the frame announces,
	// so the cap is the per-response memory an attacker can force. 64 MiB is
	// far above any page or API response and keeps that cost small.
	zstdMaxDecoded = 64 << 20
)

// zstdMagic starts every zstd frame (RFC 8878 section 3.1.1).
var zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}

// zstdDecoder is shared: DecodeAll is safe for concurrent use.
var zstdDecoder = sync.OnceValues(func() (*zstd.Decoder, error) {
	return zstd.NewReader(nil,
		zstd.WithDecoderMaxWindow(zstdMaxWindow),
		zstd.WithDecoderMaxMemory(zstdMaxDecoded),
	)
})

// decompressBody transparently decompresses gzip, deflate or zstd response bodies.
// If the content-encoding header indicates compression, the body is decompressed.
// Falls back to gzip magic-byte detection when headers are missing.
func decompressBody(data []byte, contentEncoding string) ([]byte, error) {
	return decompressBodyLimit(data, contentEncoding, 0)
}

// decompressBodyLimit is decompressBody with a bound on the decoded size:
// limit > 0 stops decoding at limit+1 bytes and returns ErrBodyTooLarge
// instead of buffering the rest. limit <= 0 means unbounded.
func decompressBodyLimit(data []byte, contentEncoding string, limit int64) ([]byte, error) {
	enc := strings.ToLower(strings.TrimSpace(contentEncoding))

	switch {
	case strings.Contains(enc, "gzip"):
		return decompressGzip(data, limit)
	case strings.Contains(enc, "deflate"):
		return decompressDeflate(data, limit)
	case strings.Contains(enc, "zstd"):
		return decompressZstd(data, limit)
	case bytes.HasPrefix(data, zstdMagic):
		// zstd frame magic — header might be stripped by proxy.
		return decompressZstd(data, limit)
	case len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b:
		// Gzip magic bytes — header might be stripped by proxy.
		return decompressGzip(data, limit)
	default:
		return data, nil
	}
}

// decodeBody turns the bytes read off the wire into the response body: it
// decompresses them and enforces limit (> 0) on the result. Both backends
// finish through it, so the size check also covers the paths that return the
// raw bytes (identity encoding, undecodable input).
func decodeBody(raw []byte, contentEncoding string, limit int64) ([]byte, error) {
	data, err := decompressBodyLimit(raw, contentEncoding, limit)
	if err != nil {
		return nil, err
	}
	if limit > 0 && int64(len(data)) > limit {
		return nil, bodyTooLarge(limit)
	}
	return data, nil
}

func decompressGzip(data []byte, limit int64) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data, nil // not valid gzip — return raw
	}
	defer r.Close()

	out, err := readAllLimit(r, limit)
	if errors.Is(err, ErrBodyTooLarge) {
		return nil, err
	}
	if err != nil {
		return data, nil // decompression failed — return raw
	}
	return out, nil
}

func decompressDeflate(data []byte, limit int64) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return data, nil
	}
	defer r.Close()

	out, err := readAllLimit(r, limit)
	if errors.Is(err, ErrBodyTooLarge) {
		return nil, err
	}
	if err != nil {
		return data, nil
	}
	return out, nil
}

func decompressZstd(data []byte, limit int64) ([]byte, error) {
	if limit > 0 {
		return decompressZstdLimit(data, limit)
	}
	dec, err := zstdDecoder()
	if err != nil {
		return data, nil
	}
	out, err := dec.DecodeAll(data, nil)
	if err != nil {
		return data, nil // not valid zstd — return raw
	}
	return out, nil
}

// decompressZstdLimit streams the frame through a decoder of its own: the
// shared one decodes a whole frame into memory (DecodeAll) before the caller
// could look at its size.
func decompressZstdLimit(data []byte, limit int64) ([]byte, error) {
	dec, err := zstd.NewReader(bytes.NewReader(data),
		zstd.WithDecoderMaxWindow(zstdMaxWindow),
		zstd.WithDecoderConcurrency(1),
	)
	if err != nil {
		return data, nil
	}
	defer dec.Close()

	out, err := readAllLimit(dec, limit)
	if errors.Is(err, ErrBodyTooLarge) {
		return nil, err
	}
	if err != nil {
		return data, nil // not valid zstd — return raw
	}
	return out, nil
}
