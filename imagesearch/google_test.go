package imagesearch

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestParseGoogleImageJSON_extracts(t *testing.T) {
	body := `)]}'
{"ischj":{"metadata":[` +
		`{"result":{"referrer_url":"https://example.com/page","page_title":"Bridge Photo"},` +
		`"original_image":{"url":"https://cdn.com/bridge.jpg","width":1200,"height":800},` +
		`"thumbnail":{"url":"https://encrypted-tbn0.gstatic.com/images?q=bridge_th"}}` +
		`,{"result":{"referrer_url":"https://other.com/sunset","page_title":"Sunset View"},` +
		`"original_image":{"url":"https://cdn.com/sunset.jpg","width":900,"height":600},` +
		`"thumbnail":{"url":"https://encrypted-tbn0.gstatic.com/images?q=sunset_th"}}` +
		`]}}`
	results := parseGoogleImageJSON([]byte(body))
	if len(results) != 2 {
		t.Fatalf("got %d, want 2", len(results))
	}
	r := results[0]
	if r.URL != "https://cdn.com/bridge.jpg" {
		t.Errorf("url = %q", r.URL)
	}
	if r.Source != "https://example.com/page" {
		t.Errorf("source = %q", r.Source)
	}
	if r.Title != "Bridge Photo" {
		t.Errorf("title = %q", r.Title)
	}
	if r.Width != 1200 || r.Height != 800 {
		t.Errorf("dims = %dx%d", r.Width, r.Height)
	}
	if r.Thumbnail != "https://encrypted-tbn0.gstatic.com/images?q=bridge_th" {
		t.Errorf("thumb = %q", r.Thumbnail)
	}
	if r.Engine != "google" {
		t.Errorf("engine = %q", r.Engine)
	}
}

func TestParseGoogleImageJSON_empty(t *testing.T) {
	if got := parseGoogleImageJSON([]byte("")); len(got) != 0 {
		t.Errorf("empty: got %d", len(got))
	}
	if got := parseGoogleImageJSON([]byte(")]}'\n{\"ischj\":{\"metadata\":[]}}")); len(got) != 0 {
		t.Errorf("empty metadata: got %d", len(got))
	}
}

func TestParseGoogleImageJSON_stripsPrefix(t *testing.T) {
	body := `)]}'
{"ischj":{"metadata":[{"result":{"referrer_url":"https://x.com","page_title":"T"},"original_image":{"url":"https://x.com/a.jpg","width":100,"height":100},"thumbnail":{"url":"https://th.com/a.jpg"}}]}}`
	results := parseGoogleImageJSON([]byte(body))
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
}

type googleHeaderCapture struct {
	headers map[string]string
}

func (g *googleHeaderCapture) Do(_, _ string, headers map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
	g.headers = headers
	return []byte(")]}'\n{\"ischj\":{\"metadata\":[]}}"), nil, 200, nil
}

func TestGoogleImages_sendsAndroidUA(t *testing.T) {
	cap := &googleHeaderCapture{}
	g := &GoogleImages{}
	_, _ = g.Search(context.Background(), cap, "test", 10)

	ua := cap.headers["user-agent"]
	if !strings.Contains(ua, "Android") || !strings.Contains(ua, "NSTN") {
		t.Errorf("UA should be Android/NSTN, got: %q", ua)
	}
}
