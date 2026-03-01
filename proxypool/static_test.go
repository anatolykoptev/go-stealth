package proxypool

import (
	"testing"
)

func TestNewStatic_SingleProxy(t *testing.T) {
	t.Parallel()

	pool := NewStatic("socks5://127.0.0.1:9050")

	if pool.Len() != 1 {
		t.Fatalf("expected Len()=1, got %d", pool.Len())
	}

	first := pool.Next()
	if first != "socks5://127.0.0.1:9050" {
		t.Fatalf("expected socks5://127.0.0.1:9050, got %s", first)
	}

	// Second call should return the same proxy (only one in pool).
	second := pool.Next()
	if second != first {
		t.Fatalf("single-proxy pool should always return the same value, got %s", second)
	}
}

func TestNewStatic_MultipleProxies(t *testing.T) {
	t.Parallel()

	pool := NewStatic(
		"socks5://127.0.0.1:9050",
		"socks5://127.0.0.1:9051",
		"socks5://127.0.0.1:9052",
	)

	if pool.Len() != 3 {
		t.Fatalf("expected Len()=3, got %d", pool.Len())
	}

	first := pool.Next()
	second := pool.Next()
	if first == second {
		t.Fatal("round-robin should return different proxies for consecutive calls")
	}

	third := pool.Next()
	fourth := pool.Next()

	// After 3 proxies, the 4th should wrap back to the 1st.
	if fourth != first {
		t.Fatalf("expected rotation back to first proxy, got %s vs %s", first, fourth)
	}

	_ = third // used to advance the counter
}

func TestNewStatic_Empty(t *testing.T) {
	t.Parallel()

	pool := NewStatic()

	if pool.Len() != 0 {
		t.Fatalf("expected Len()=0, got %d", pool.Len())
	}

	next := pool.Next()
	if next != "" {
		t.Fatalf("expected empty string from empty pool, got %s", next)
	}
}

func TestStatic_TransportProxy(t *testing.T) {
	t.Parallel()

	pool := NewStatic("socks5://127.0.0.1:9050")

	fn := pool.TransportProxy()
	if fn == nil {
		t.Fatal("TransportProxy should not return nil")
	}

	proxyURL, err := fn(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proxyURL.Hostname() != "127.0.0.1" {
		t.Fatalf("expected host 127.0.0.1, got %s", proxyURL.Hostname())
	}
	if proxyURL.Scheme != "socks5" {
		t.Fatalf("expected scheme socks5, got %s", proxyURL.Scheme)
	}
}

func TestStatic_TransportProxy_Empty(t *testing.T) {
	t.Parallel()

	pool := NewStatic()

	fn := pool.TransportProxy()
	proxyURL, err := fn(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proxyURL != nil {
		t.Fatalf("expected nil URL from empty pool, got %v", proxyURL)
	}
}
