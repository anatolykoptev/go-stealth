package webextract

import (
	"strings"
	"testing"
)

const sampleHTML = `<!DOCTYPE html>
<html>
<head><title>Test Article</title></head>
<body>
<header><nav>Navigation</nav></header>
<main>
<article>
<h1>Test Article Title</h1>
<p>This is the first paragraph of the article.</p>
<p>Second paragraph with more details.</p>
</article>
</main>
<footer>Footer content</footer>
<script>var x = 1;</script>
<style>.hidden { display: none; }</style>
</body>
</html>`

func TestExtractContent_BasicArticle(t *testing.T) {
	t.Parallel()
	result := ExtractContent(sampleHTML, 0)
	if !strings.Contains(result, "first paragraph") {
		t.Errorf("expected article text, got: %q", result)
	}
}

func TestExtractContent_BoilerplateRemoval(t *testing.T) {
	t.Parallel()
	result := ExtractContent(sampleHTML, 0)
	for _, unwanted := range []string{"Navigation", "Footer content", "var x"} {
		if strings.Contains(result, unwanted) {
			t.Errorf("content should not contain %q", unwanted)
		}
	}
}

func TestExtractContent_ExtendedSelectors(t *testing.T) {
	t.Parallel()
	html := `<html><body>
		<div class="breadcrumbs">Home > News</div>
		<div class="widget">Popular posts</div>
		<div class="author-info">Written by John</div>
		<div class="banner">Subscribe now!</div>
		<article><p>Actual article content here.</p></article>
	</body></html>`

	result := ExtractContent(html, 0)
	for _, unwanted := range []string{"Home > News", "Popular posts", "Written by John", "Subscribe now"} {
		if strings.Contains(result, unwanted) {
			t.Errorf("content should not contain %q", unwanted)
		}
	}
	if !strings.Contains(result, "Actual article content") {
		t.Errorf("expected article content, got: %q", result)
	}
}

func TestExtractContent_MaxLen(t *testing.T) {
	t.Parallel()
	result := ExtractContent(sampleHTML, 20)
	if len(result) > 20 {
		t.Errorf("content length %d exceeds maxLen 20", len(result))
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestExtractContent_MaxLenUTF8(t *testing.T) {
	t.Parallel()
	// Russian text: each Cyrillic char is 2 bytes in UTF-8.
	html := `<html><body><p>Привет мир</p></body></html>`
	result := ExtractContent(html, 7)
	// Should not cut in the middle of a multi-byte char.
	for i := range result {
		if result[i]&0xC0 == 0x80 && i == len(result)-1 {
			t.Error("truncated in middle of UTF-8 character")
		}
		_ = i
	}
	if len(result) > 7 {
		t.Errorf("length %d exceeds maxLen 7", len(result))
	}
}

func TestExtractContent_EmptyHTML(t *testing.T) {
	t.Parallel()
	result := ExtractContent("", 0)
	if result != "" {
		t.Errorf("expected empty result for empty HTML, got: %q", result)
	}
}

func TestExtractContent_ContentPriority(t *testing.T) {
	t.Parallel()
	// article should be preferred over body.
	html := `<html><body>
		<p>Body text outside article.</p>
		<article><p>Article text inside.</p></article>
	</body></html>`
	result := ExtractContent(html, 0)
	if !strings.Contains(result, "Article text inside") {
		t.Errorf("expected article content, got: %q", result)
	}
	if strings.Contains(result, "Body text outside") {
		t.Errorf("should prefer article over body, got: %q", result)
	}
}

func TestExtractContent_FallbackToBody(t *testing.T) {
	t.Parallel()
	html := `<html><body><p>Body content only.</p></body></html>`
	result := ExtractContent(html, 0)
	if !strings.Contains(result, "Body content only") {
		t.Errorf("expected body fallback, got: %q", result)
	}
}

func TestExtractContent_HiddenElements(t *testing.T) {
	t.Parallel()
	html := `<html><body>
		<div aria-hidden="true">Hidden from screen readers</div>
		<div hidden>HTML5 hidden</div>
		<div class="cookie-banner">Accept cookies</div>
		<p>Visible content</p>
	</body></html>`
	result := ExtractContent(html, 0)
	for _, unwanted := range []string{"Hidden from screen readers", "HTML5 hidden", "Accept cookies"} {
		if strings.Contains(result, unwanted) {
			t.Errorf("content should not contain %q", unwanted)
		}
	}
	if !strings.Contains(result, "Visible content") {
		t.Error("visible content should be preserved")
	}
}
