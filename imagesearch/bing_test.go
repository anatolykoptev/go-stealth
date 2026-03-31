package imagesearch

import "testing"

func TestParseBingHTML_extractsImages(t *testing.T) {
	html := `<div class="imgpt"><a m="{&quot;murl&quot;:&quot;https://example.com/photo.jpg&quot;,&quot;turl&quot;:&quot;https://th.bing.com/th1.jpg&quot;,&quot;purl&quot;:&quot;https://example.com/page&quot;,&quot;t&quot;:&quot;Nice Photo&quot;}"></a></div>` +
		`<div class="imgpt"><a m="{&quot;murl&quot;:&quot;https://other.com/cat.png&quot;,&quot;turl&quot;:&quot;https://th.bing.com/th2.jpg&quot;,&quot;purl&quot;:&quot;https://other.com/cats&quot;,&quot;t&quot;:&quot;Cat&quot;}"></a></div>`
	results := parseBingHTML(html)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].URL != "https://example.com/photo.jpg" {
		t.Errorf("url[0] = %q", results[0].URL)
	}
	if results[0].Thumbnail != "https://th.bing.com/th1.jpg" {
		t.Errorf("thumb[0] = %q", results[0].Thumbnail)
	}
	if results[0].Source != "https://example.com/page" {
		t.Errorf("source[0] = %q", results[0].Source)
	}
	if results[0].Title != "Nice Photo" {
		t.Errorf("title[0] = %q", results[0].Title)
	}
	if results[0].Engine != "bing" {
		t.Errorf("engine[0] = %q", results[0].Engine)
	}
}

func TestParseBingHTML_empty(t *testing.T) {
	if got := parseBingHTML(""); len(got) != 0 {
		t.Errorf("empty: got %d", len(got))
	}
	if got := parseBingHTML("<html><body>no images</body></html>"); len(got) != 0 {
		t.Errorf("no images: got %d", len(got))
	}
}

func TestParseBingHTML_malformedJSON(t *testing.T) {
	html := `<a m="{not valid json}"></a>`
	if got := parseBingHTML(html); len(got) != 0 {
		t.Errorf("malformed: got %d", len(got))
	}
}

func TestParseBingHTML_missingMurl(t *testing.T) {
	html := `<a m="{&quot;turl&quot;:&quot;https://th.jpg&quot;}"></a>`
	if got := parseBingHTML(html); len(got) != 0 {
		t.Errorf("no murl: got %d", len(got))
	}
}

func TestParseBingHTML_extractsDimensions(t *testing.T) {
	html := `<a m="{&quot;murl&quot;:&quot;https://example.com/photo.jpg&quot;,&quot;turl&quot;:&quot;https://th.bing.com/th1.jpg&quot;,&quot;purl&quot;:&quot;https://example.com/page&quot;,&quot;t&quot;:&quot;Photo&quot;,&quot;mw&quot;:&quot;1920&quot;,&quot;mh&quot;:&quot;1080&quot;}"></a>`
	results := parseBingHTML(html)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Width != 1920 || results[0].Height != 1080 {
		t.Errorf("dims = %dx%d, want 1920x1080", results[0].Width, results[0].Height)
	}
}
