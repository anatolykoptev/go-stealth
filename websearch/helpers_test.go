package websearch

import (
	"net/http"
	"testing"
)

func TestChromeHeaders(t *testing.T) {
	h := ChromeHeaders()
	if len(h) == 0 {
		t.Fatal("ChromeHeaders returned empty map")
	}
	if h["user-agent"] == "" {
		t.Error("ChromeHeaders missing user-agent")
	}
	if h["accept"] == "" {
		t.Error("ChromeHeaders missing accept")
	}
}

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"bold tags", "Example <b>Title</b>", "Example Title"},
		{"em tags", "Some <em>content</em> here", "Some content here"},
		{"nested tags", "<div><a href='x'>link</a></div>", "link"},
		{"empty", "", ""},
		{"whitespace", "  <b> text </b>  ", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanHTML(tt.input)
			if got != tt.want {
				t.Errorf("CleanHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsRateLimitStatus(t *testing.T) {
	if !isRateLimitStatus(http.StatusTooManyRequests) {
		t.Error("expected true for 429")
	}
	if !isRateLimitStatus(http.StatusForbidden) {
		t.Error("expected true for 403")
	}
	if isRateLimitStatus(http.StatusOK) {
		t.Error("expected false for 200")
	}
}
