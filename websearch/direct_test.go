package websearch

import (
	"context"
	"io"
	"testing"
)

// directMockBrowser returns canned responses for direct_test.
type directMockBrowser struct {
	handler func(method, url string) ([]byte, int)
}

func (m *directMockBrowser) Do(method, url string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
	body, status := m.handler(method, url)
	return body, nil, status, nil
}

func TestSearchDirect_NilBrowser(t *testing.T) {
	results := SearchDirect(context.Background(), DirectConfig{}, "test", "en")
	if results != nil {
		t.Errorf("expected nil with nil browser, got %d results", len(results))
	}
}

func TestSearchDirect_NoEnginesEnabled(t *testing.T) {
	cfg := DirectConfig{
		Browser: &directMockBrowser{handler: func(_, _ string) ([]byte, int) {
			return nil, 200
		}},
	}
	results := SearchDirect(context.Background(), cfg, "test", "en")
	if len(results) != 0 {
		t.Errorf("expected 0 results with no engines, got %d", len(results))
	}
}

func TestSearchDirect_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := DirectConfig{
		Browser: &directMockBrowser{handler: func(_, _ string) ([]byte, int) {
			return nil, 200
		}},
		DDG:       true,
		Startpage: true,
	}
	// Should not hang — context already cancelled.
	results := SearchDirect(ctx, cfg, "test", "en")
	// Results may be nil or empty, both are fine.
	_ = results
}
