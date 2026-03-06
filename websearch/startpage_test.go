package websearch

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestParseStartpageHTML(t *testing.T) {
	html := `<html><body>
		<div class="w-gl__result">
			<a class="w-gl__result-title" href="https://example.com/sp1">SP Result 1</a>
			<p class="w-gl__description">Startpage snippet 1.</p>
		</div>
		<div class="w-gl__result">
			<a class="w-gl__result-title" href="https://example.com/sp2">SP Result 2</a>
			<p class="w-gl__description">Startpage snippet 2.</p>
		</div>
		<div class="w-gl__result">
			<a class="w-gl__result-title" href="https://www.startpage.com/do/proxy">Ad Result</a>
			<p class="w-gl__description">This is an ad.</p>
		</div>
	</body></html>`

	results, err := ParseStartpageHTML([]byte(html))
	if err != nil {
		t.Fatalf("ParseStartpageHTML: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (ad filtered)", len(results))
	}
	if results[0].Title != "SP Result 1" {
		t.Errorf("results[0].Title = %q, want SP Result 1", results[0].Title)
	}
	if results[0].Content != "Startpage snippet 1." {
		t.Errorf("results[0].Content = %q, want 'Startpage snippet 1.'", results[0].Content)
	}
	if results[0].Score != 1.0 {
		t.Errorf("results[0].Score = %f, want 1.0", results[0].Score)
	}
	if results[0].Metadata["engine"] != "startpage" {
		t.Errorf("results[0].Metadata[engine] = %q, want startpage", results[0].Metadata["engine"])
	}
}

func TestIsStartpageRateLimited(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"normal response", "<html><body>results</body></html>", false},
		{"rate limited", "<html>You have been rate limited</html>", true},
		{"too many requests", "<html>Too Many Requests</html>", true},
		{"captcha", "<html><div class=\"g-recaptcha\"></div></html>", true},
		{"generic captcha", "<html>Please solve the captcha</html>", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStartpageRateLimited([]byte(tt.body))
			if got != tt.want {
				t.Errorf("isStartpageRateLimited() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartpage_Search_Mock(t *testing.T) {
	htmlResponse := `<html><body>
		<div class="w-gl__result">
			<a class="w-gl__result-title" href="https://example.com/sp">SP Result</a>
			<p class="w-gl__description">SP description.</p>
		</div>
	</body></html>`

	bc := &mockBrowser{fn: func(method, u string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
		if method != http.MethodPost {
			t.Errorf("method = %q, want POST", method)
		}
		if !strings.Contains(u, "startpage.com/sp/search") {
			t.Errorf("url = %q, want startpage search URL", u)
		}
		return []byte(htmlResponse), nil, http.StatusOK, nil
	}}

	sp := NewStartpage(WithStartpageBrowser(bc))

	results, err := sp.Search(context.Background(), "test", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Title != "SP Result" {
		t.Errorf("results[0].Title = %q, want SP Result", results[0].Title)
	}
}

func TestStartpage_NoBrowser(t *testing.T) {
	sp := NewStartpage()
	_, err := sp.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Fatal("expected error when no BrowserDoer provided")
	}
}
