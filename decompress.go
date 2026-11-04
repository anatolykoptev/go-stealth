package stealth

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

const (
	// zstdMaxWindow is the largest window a zstd content coding may use
	// (RFC 9659 section 3); a bigger one is not a valid HTTP body.
	zstdMaxWindow = 8 << 20
	// zstdMaxDecoded bounds one decoded body so a small hostile response
	// cannot expand without limit.
	zstdMaxDecoded = 512 << 20
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
	enc := strings.ToLower(strings.TrimSpace(contentEncoding))

	switch {
	case strings.Contains(enc, "gzip"):
		return decompressGzip(data)
	case strings.Contains(enc, "deflate"):
		return decompressDeflate(data)
	case strings.Contains(enc, "zstd"):
		return decompressZstd(data)
	case bytes.HasPrefix(data, zstdMagic):
		// zstd frame magic — header might be stripped by proxy.
		return decompressZstd(data)
	case len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b:
		// Gzip magic bytes — header might be stripped by proxy.
		return decompressGzip(data)
	default:
		return data, nil
	}
}

func decompressGzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data, nil // not valid gzip — return raw
	}
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		return data, nil // decompression failed — return raw
	}
	return out, nil
}

func decompressDeflate(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return data, nil
	}
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		return data, nil
	}
	return out, nil
}

func decompressZstd(data []byte) ([]byte, error) {
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
