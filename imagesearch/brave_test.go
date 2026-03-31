package imagesearch

import "testing"

func TestParseBraveImageHTML_extracts(t *testing.T) {
	html := `<script>{"url":"https://page.com/photo","title":"Beach Sunset","properties":{"url":"https://cdn.com/sunset.jpg","width":1920,"height":1080},"thumbnail":{"src":"https://cdn.com/sunset_thumb.jpg"}}</script>`
	results := parseBraveImageHTML(html)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	r := results[0]
	if r.URL != "https://cdn.com/sunset.jpg" {
		t.Errorf("url = %q", r.URL)
	}
	if r.Source != "https://page.com/photo" {
		t.Errorf("source = %q", r.Source)
	}
	if r.Thumbnail != "https://cdn.com/sunset_thumb.jpg" {
		t.Errorf("thumb = %q", r.Thumbnail)
	}
	if r.Title != "Beach Sunset" {
		t.Errorf("title = %q", r.Title)
	}
	if r.Width != 1920 || r.Height != 1080 {
		t.Errorf("dims = %dx%d", r.Width, r.Height)
	}
	if r.Engine != "brave" {
		t.Errorf("engine = %q", r.Engine)
	}
}

func TestParseBraveImageHTML_empty(t *testing.T) {
	if got := parseBraveImageHTML(""); len(got) != 0 {
		t.Errorf("empty: got %d", len(got))
	}
}

func TestParseBraveImageHTML_missingProperties(t *testing.T) {
	html := `{"url":"https://page.com","title":"Test","thumbnail":{"src":"t.jpg"}}`
	if got := parseBraveImageHTML(html); len(got) != 0 {
		t.Errorf("no properties: got %d", len(got))
	}
}
