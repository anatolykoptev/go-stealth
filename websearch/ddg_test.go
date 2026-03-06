package websearch

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// mockBrowser implements BrowserDoer for testing.
type mockBrowser struct {
	fn func(method, url string, headers map[string]string, body io.Reader) ([]byte, map[string]string, int, error)
}

func (m *mockBrowser) Do(method, url string, headers map[string]string, body io.Reader) ([]byte, map[string]string, int, error) {
	return m.fn(method, url, headers, body)
}

func TestParseDDGHTML(t *testing.T) {
	html := `<html><body>
		<div class="result">
			<a class="result__a" href="https://example.com/page1">Example Page</a>
			<span class="result__snippet">This is a snippet.</span>
		</div>
		<div class="result">
			<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fother.com%2Fpage&rut=abc">Other Page</a>
			<span class="result__snippet">Another snippet.</span>
		</div>
	</body></html>`

	results, err := ParseDDGHTML([]byte(html))
	if err != nil {
		t.Fatalf("ParseDDGHTML: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Title != "Example Page" {
		t.Errorf("results[0].Title = %q, want Example Page", results[0].Title)
	}
	if results[0].URL != "https://example.com/page1" {
		t.Errorf("results[0].URL = %q, want https://example.com/page1", results[0].URL)
	}
	if results[1].URL != "https://other.com/page" {
		t.Errorf("results[1].URL = %q, want https://other.com/page (unwrapped)", results[1].URL)
	}
	if results[0].Score != 1.0 {
		t.Errorf("results[0].Score = %f, want 1.0", results[0].Score)
	}
	if results[0].Metadata["engine"] != "ddg" {
		t.Errorf("results[0].Metadata[engine] = %q, want ddg", results[0].Metadata["engine"])
	}
}

func TestDDGUnwrapURL(t *testing.T) {
	tests := []struct {
		name string
		href string
		want string
	}{
		{"direct URL", "https://example.com/page", "https://example.com/page"},
		{"wrapped URL", "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage&rut=abc", "https://example.com/page"},
		{"no uddg param", "//duckduckgo.com/l/?foo=bar", ""},
		{"relative URL", "/some/path", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DDGUnwrapURL(tt.href)
			if got != tt.want {
				t.Errorf("DDGUnwrapURL(%q) = %q, want %q", tt.href, got, tt.want)
			}
		})
	}
}

func TestParseDDGResponse(t *testing.T) {
	jsonResp := `[
		{"t": "Example <b>Title</b>", "a": "Some <em>content</em>", "u": "https://example.com/1", "c": ""},
		{"t": "Other", "a": "Text", "u": "", "c": "https://other.com"},
		{"t": "", "a": "No title", "u": "https://skip.com", "c": ""},
		{"t": "DDG Internal", "a": "", "u": "https://duckduckgo.com/about", "c": ""}
	]`

	results, err := ParseDDGResponse([]byte(jsonResp))
	if err != nil {
		t.Fatalf("ParseDDGResponse: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Title != "Example Title" {
		t.Errorf("results[0].Title = %q, want 'Example Title' (HTML stripped)", results[0].Title)
	}
	if results[1].URL != "https://other.com" {
		t.Errorf("results[1].URL = %q, want https://other.com (from C field)", results[1].URL)
	}
}

func TestParseDDGResponse_JSONP(t *testing.T) {
	jsonp := `DDGjsonp_abc123([{"t":"Title","a":"Content","u":"https://example.com","c":""}])`
	results, err := ParseDDGResponse([]byte(jsonp))
	if err != nil {
		t.Fatalf("ParseDDGResponse JSONP: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
}

func TestExtractVQD(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"single quotes", `<script>vqd='abc123'</script>`, "abc123"},
		{"double quotes", `<script>vqd="def456"</script>`, "def456"},
		{"no quotes", `vqd=xyz789-_test`, "xyz789-_test"},
		{"not found", `<html>no vqd here</html>`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractVQD(tt.body)
			if got != tt.want {
				t.Errorf("ExtractVQD() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsDDGRateLimited(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"normal response", "<html><body>results</body></html>", false},
		{"please try again", "<html>Please Try Again later</html>", true},
		{"captcha form", `<form action="/d.js"><input type="hidden" /></form>`, true},
		{"blocked", "<html>Your request has been blocked</html>", true},
		{"unusual traffic", "<html>unusual traffic detected</html>", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDDGRateLimited([]byte(tt.body))
			if got != tt.want {
				t.Errorf("isDDGRateLimited() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDDG_Search_Mock(t *testing.T) {
	htmlResponse := `<html><body>
		<div class="result">
			<a class="result__a" href="https://example.com/ddg1">DDG Result</a>
			<span class="result__snippet">DDG snippet.</span>
		</div>
	</body></html>`

	bc := &mockBrowser{fn: func(_, _ string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
		return []byte(htmlResponse), nil, http.StatusOK, nil
	}}

	ddg, err := NewDDG(WithDDGBrowser(bc))
	if err != nil {
		t.Fatalf("NewDDG: %v", err)
	}

	results, err := ddg.Search(context.Background(), "test", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Title != "DDG Result" {
		t.Errorf("results[0].Title = %q, want DDG Result", results[0].Title)
	}
}

func TestNewDDG_NoBrowser(t *testing.T) {
	_, err := NewDDG()
	if err == nil {
		t.Fatal("expected error when no BrowserDoer provided")
	}
}
