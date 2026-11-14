package stealth

import (
	"errors"
	"fmt"
	"io"
	"math"
)

// ErrBodyTooLarge is returned (wrapped; match it with errors.Is) when a
// response body is larger than the limit set with WithMaxBodyBytes. The limit
// applies to the body after content-decoding, and reading stops one byte past
// it: the rest of the body is neither buffered nor drained, and the
// connection is closed rather than reused.
var ErrBodyTooLarge = errors.New("stealth: response body exceeds the configured limit")

func bodyTooLarge(limit int64) error {
	return fmt.Errorf("%w (%d bytes)", ErrBodyTooLarge, limit)
}

// readAllLimit is io.ReadAll that stops at limit+1 bytes and reports
// ErrBodyTooLarge when the source has more than limit bytes. limit <= 0 reads
// without a bound.
func readAllLimit(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(r)
	}
	n := limit
	if n < math.MaxInt64 {
		n++
	}
	b, err := io.ReadAll(io.LimitReader(r, n))
	if int64(len(b)) > limit {
		return nil, bodyTooLarge(limit)
	}
	return b, err
}

// ReadBodyLimit is io.ReadAll bounded by limit, for a custom backend (see
// WithBackend) to read its response body with: it stops at limit+1 bytes and
// returns an error wrapping ErrBodyTooLarge when the source has more than limit
// bytes, without buffering the rest. Pass BackendConfig.MaxBodyBytes as the
// limit; a limit <= 0 reads without a bound. Close the body unread afterwards,
// so the connection is dropped rather than drained.
func ReadBodyLimit(r io.Reader, limit int64) ([]byte, error) {
	return readAllLimit(r, limit)
}

// wireLimit is how many bytes to read off the wire for a body whose decoded
// size must not pass limit. An identity body, and one in a coding this package
// does not decode (br, an unknown name), is its own decoded form, so the limit
// applies as is. A body in a coding it does decode (gzip, deflate, zstd) gets a
// small allowance for the coding's framing, so that an incompressible body of
// exactly limit bytes is not refused for its encoder's overhead; decodeBody
// then enforces limit on the decoded bytes.
func wireLimit(limit int64, contentEncoding string) int64 {
	if limit <= 0 {
		return 0
	}
	if !decodesEncoding(contentEncoding) {
		return limit
	}
	return satAdd(limit, limit>>8+1024)
}

// oxEnvelopeLimit bounds the JSON reply that carries a page body back from
// ox-browser: JSON escapes at most six bytes per body byte, plus the other
// fields of the reply.
func oxEnvelopeLimit(limit int64) int64 {
	if limit <= 0 {
		return 0
	}
	if limit > (math.MaxInt64-1<<20)/6 {
		return math.MaxInt64
	}
	return limit*6 + 1<<20
}

func satAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}
