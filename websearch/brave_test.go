package websearch

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
)

func TestParseBraveHTML(t *testing.T) {
	data, err := os.ReadFile("testdata/brave_search.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	results, err := ParseBraveHTML(data)
	if err != nil {
		t.Fatalf("ParseBraveHTML: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result, got 0")
	}

	for i, r := range results {
		if r.Title == "" {
			t.Errorf("results[%d].Title is empty", i)
		}
		if r.URL == "" {
			t.Errorf("results[%d].URL is empty", i)
		}
		if r.Score != directResultScore {
			t.Errorf("results[%d].Score = %f, want %f", i, r.Score, directResultScore)
		}
		if r.Metadata["engine"] != "brave" {
			t.Errorf("results[%d].Metadata[engine] = %q, want brave", i, r.Metadata["engine"])
		}
	}

	t.Logf("parsed %d results from fixture", len(results))
	if len(results) > 0 {
		t.Logf("first result: title=%q url=%q", results[0].Title, results[0].URL)
	}
}

func TestBrave_RateLimitStatus(t *testing.T) {
	bc := &mockBrowser{fn: func(_, _ string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
		return []byte("error"), nil, http.StatusTooManyRequests, nil
	}}

	b := NewBrave(WithBraveBrowser(bc))
	_, err := b.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Fatal("expected error on 429 status")
	}
	rl, ok := err.(*ErrRateLimited)
	if !ok {
		t.Fatalf("expected ErrRateLimited, got %T: %v", err, err)
	}
	if rl.Engine != "brave" {
		t.Errorf("Engine = %q, want brave", rl.Engine)
	}
}

func TestBrave_RateLimitBody(t *testing.T) {
	bc := &mockBrowser{fn: func(_, _ string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
		return []byte("<html>Please solve the captcha to continue</html>"), nil, http.StatusOK, nil
	}}

	b := NewBrave(WithBraveBrowser(bc))
	_, err := b.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Fatal("expected error on captcha body")
	}
	rl, ok := err.(*ErrRateLimited)
	if !ok {
		t.Fatalf("expected ErrRateLimited, got %T: %v", err, err)
	}
	if rl.Engine != "brave" {
		t.Errorf("Engine = %q, want brave", rl.Engine)
	}
}

func TestBrave_NoBrowser(t *testing.T) {
	b := NewBrave()
	_, err := b.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Fatal("expected error when no BrowserDoer provided")
	}
}

func TestBrave_Search_Mock(t *testing.T) {
	html := `<html><body>
		<div data-pos="1">
			<a href="https://example.com/go">
				<div class="title">Go Programming</div>
			</a>
			<div class="content t-primary">Go is a programming language.</div>
		</div>
	</body></html>`

	bc := &mockBrowser{fn: func(method, u string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
		if method != http.MethodGet {
			t.Errorf("method = %q, want GET", method)
		}
		return []byte(html), nil, http.StatusOK, nil
	}}

	b := NewBrave(WithBraveBrowser(bc))
	results, err := b.Search(context.Background(), "golang", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Title != "Go Programming" {
		t.Errorf("Title = %q, want Go Programming", results[0].Title)
	}
	if results[0].URL != "https://example.com/go" {
		t.Errorf("URL = %q, want https://example.com/go", results[0].URL)
	}
}
