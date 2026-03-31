package imagesearch

import "testing"

func TestParseBraveImageHTML_extracts(t *testing.T) {
	// Brave SSR uses JS object notation (unquoted keys), not JSON.
	html := `<script>{url:"https://page.com/photo",title:"Beach Sunset",` +
		`properties:{url:"https://cdn.com/sunset.jpg",resized:"r.jpg",height:1080,width:1920,` +
		`format:"jpeg"},thumbnail:{src:"https://cdn.com/sunset_thumb.jpg"}}</script>`
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
	html := `{url:"https://page.com",title:"Test",thumbnail:{src:"t.jpg"}}`
	if got := parseBraveImageHTML(html); len(got) != 0 {
		t.Errorf("no properties: got %d", len(got))
	}
}

func TestParseBraveImageHTML_realWorldJSObject(t *testing.T) {
	html := `<div id="results">` +
		`{url:"https://page.com/gallery",title:"Bridge Photo",` +
		`properties:{url:"https://cdn.com/bridge.jpg",resized:"https://cdn.com/bridge_r.jpg",height:800,width:1200,` +
		`format:"jpeg"},thumbnail:{src:"https://cdn.com/bridge_th.jpg"}}` +
		`,{url:"https://other.com/sunset",title:"Sunset View",` +
		`properties:{url:"https://cdn.com/sunset.jpg",resized:"https://cdn.com/sunset_r.jpg",height:600,width:900,` +
		`format:"png"},thumbnail:{src:"https://cdn.com/sunset_th.jpg"}}` +
		`</div>`
	results := parseBraveImageHTML(html)
	if len(results) != 2 {
		t.Fatalf("got %d, want 2", len(results))
	}
	r := results[0]
	if r.URL != "https://cdn.com/bridge.jpg" {
		t.Errorf("url = %q, want bridge.jpg", r.URL)
	}
	if r.Width != 1200 || r.Height != 800 {
		t.Errorf("dims = %dx%d, want 1200x800", r.Width, r.Height)
	}
	if r.Source != "https://page.com/gallery" {
		t.Errorf("source = %q", r.Source)
	}
	if r.Title != "Bridge Photo" {
		t.Errorf("title = %q", r.Title)
	}
	if r.Thumbnail != "https://cdn.com/bridge_th.jpg" {
		t.Errorf("thumb = %q", r.Thumbnail)
	}
}

func TestParseBraveImageHTML_escapedURL(t *testing.T) {
	html := `{url:"https://page.com/x",title:"Test",` +
		`properties:{url:"https://cdn.com/img.jpg?w=100\u0026h=200",resized:"r.jpg",height:200,width:100,` +
		`format:"jpeg"},thumbnail:{src:"https://cdn.com/th.jpg"}}`
	results := parseBraveImageHTML(html)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].URL == "" {
		t.Error("URL should not be empty")
	}
}
