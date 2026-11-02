package stealth

import (
	"errors"
	"sync/atomic"
	"testing"

	tls_client "github.com/bogdanfinn/tls-client"
)

// closeRecorder is a tls_client.HttpClient whose only implemented method is
// Close; any other call panics on the nil embedded interface.
type closeRecorder struct {
	tls_client.HttpClient
	closes atomic.Int32
	idle   atomic.Int32
}

func (c *closeRecorder) Close()                { c.closes.Add(1) }
func (c *closeRecorder) CloseIdleConnections() { c.idle.Add(1) }

// recordingTLS installs rec as the client's tls-client, per client (no global).
func recordingTLS(rec *closeRecorder) ClientOption {
	return func(c *clientConfig) {
		c.newHTTPClient = func(tls_client.Logger, ...tls_client.HttpClientOption) (tls_client.HttpClient, error) {
			return rec, nil
		}
	}
}

func TestBrowserClient_Close_ClosesBuiltTLSClientOnce(t *testing.T) {
	rec := &closeRecorder{}
	bc, err := NewClient(recordingTLS(rec))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := bc.Close(); err != nil {
			t.Fatalf("Close #%d: %v", i, err)
		}
	}
	if got := rec.closes.Load(); got != 1 {
		t.Fatalf("tls-client Close called %d times, want exactly 1", got)
	}
}

type closableBackend struct {
	closes atomic.Int32
}

func (b *closableBackend) Do(*Request) (*Response, error) { return &Response{}, nil }
func (b *closableBackend) SetProxy(string) error          { return nil }
func (b *closableBackend) GetCookieValue(string, string) string {
	return ""
}
func (b *closableBackend) Close() error { b.closes.Add(1); return nil }

func TestBrowserClient_Close_LeavesCallerBackendAlone(t *testing.T) {
	be := &closableBackend{}
	bc, err := NewClient(WithBackend(func(BackendConfig) (HTTPDoer, error) { return be, nil }))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = bc.Close()
	if got := be.closes.Load(); got != 0 {
		t.Fatalf("caller-supplied backend closed %d times, want 0", got)
	}
}

// A caller-supplied backend that IS a *tlsClientDoer (possible in-package)
// is still the caller's.
func TestBrowserClient_Close_LeavesCallerTLSDoerAlone(t *testing.T) {
	rec := &closeRecorder{}
	bc, err := NewClient(WithBackend(func(BackendConfig) (HTTPDoer, error) {
		return &tlsClientDoer{client: rec}, nil
	}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = bc.Close()
	if got := rec.closes.Load(); got != 0 {
		t.Fatalf("caller-supplied tls doer closed %d times, want 0", got)
	}
}

type errCloser struct{ err error }

func (e errCloser) Close() error { return e.err }

func TestBrowserClient_Close_PoolErrorStillReturnedAndDoerClosed(t *testing.T) {
	rec := &closeRecorder{}
	want := errors.New("pool close failed")
	bc, err := NewClient(recordingTLS(rec), func(c *clientConfig) { c.poolCloser = errCloser{want} })
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := bc.Close(); !errors.Is(got, want) {
		t.Fatalf("Close() = %v, want %v", got, want)
	}
	if got := bc.Close(); !errors.Is(got, want) {
		t.Fatalf("second Close() = %v, want sticky %v", got, want)
	}
	if got := rec.closes.Load(); got != 1 {
		t.Fatalf("doer closed %d times, want 1", got)
	}
}

// WithBackend(nil) means "default backend": the client builds it, so it owns it.
func TestBrowserClient_Close_WithBackendNilIsOwned(t *testing.T) {
	rec := &closeRecorder{}
	bc, err := NewClient(WithBackend(nil), recordingTLS(rec))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = bc.Close()
	if got := rec.closes.Load(); got != 1 {
		t.Fatalf("default backend via WithBackend(nil) closed %d times, want 1", got)
	}
}
