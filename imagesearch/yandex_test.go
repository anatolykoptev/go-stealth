package imagesearch

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestParseYandexDataState_extracts(t *testing.T) {
	// Multiple data-state blocks — parser must find the one with initialState.serpList
	html := `<div data-state="{&quot;form&quot;:{}}"></div>` +
		`<div data-state="{&quot;initialState&quot;:{&quot;serpList&quot;:{&quot;items&quot;:{&quot;entities&quot;:{&quot;abc123&quot;:{&quot;origUrl&quot;:&quot;https://img.com/photo.jpg&quot;,&quot;image&quot;:&quot;//avatars.mds.yandex.net/i?id=abc&quot;,&quot;origWidth&quot;:1200,&quot;origHeight&quot;:800,&quot;snippet&quot;:{&quot;title&quot;:&quot;Nice Photo&quot;,&quot;domain&quot;:&quot;example.com&quot;},&quot;url&quot;:&quot;/images/search?pos=0&amp;img_url=https%3A%2F%2Fexample.com%2Fpage&quot;},&quot;def456&quot;:{&quot;origUrl&quot;:&quot;https://img.com/cat.jpg&quot;,&quot;image&quot;:&quot;//avatars.mds.yandex.net/i?id=def&quot;,&quot;origWidth&quot;:800,&quot;origHeight&quot;:600,&quot;snippet&quot;:{&quot;title&quot;:&quot;Cat&quot;,&quot;domain&quot;:&quot;cats.com&quot;},&quot;url&quot;:&quot;/images/search?pos=1&amp;img_url=https%3A%2F%2Fcats.com%2Fpage&quot;}}}}}}" ></div>`
	results := parseYandexDataState(html)
	if len(results) != 2 {
		t.Fatalf("got %d, want 2", len(results))
	}

	found := false
	for _, r := range results {
		if r.URL == "https://img.com/photo.jpg" {
			found = true
			if r.Engine != "yandex" {
				t.Errorf("engine = %q", r.Engine)
			}
			if r.Thumbnail != "https://avatars.mds.yandex.net/i?id=abc" {
				t.Errorf("thumb = %q", r.Thumbnail)
			}
			if r.Title != "Nice Photo" {
				t.Errorf("title = %q", r.Title)
			}
			if r.Width != 1200 || r.Height != 800 {
				t.Errorf("dims = %dx%d", r.Width, r.Height)
			}
			if r.Source != "https://example.com/page" {
				t.Errorf("source = %q", r.Source)
			}
		}
	}
	if !found {
		t.Error("expected to find https://img.com/photo.jpg")
	}
}

func TestParseYandexDataState_empty(t *testing.T) {
	if got := parseYandexDataState(""); len(got) != 0 {
		t.Errorf("empty: got %d", len(got))
	}
	if got := parseYandexDataState("<html>no data-state</html>"); len(got) != 0 {
		t.Errorf("no state: got %d", len(got))
	}
}

func TestParseYandexDataState_skipsFormBlock(t *testing.T) {
	html := `<div data-state="{&quot;form&quot;:{&quot;action&quot;:&quot;/images/search&quot;}}"></div>`
	if got := parseYandexDataState(html); len(got) != 0 {
		t.Errorf("form only: got %d", len(got))
	}
}

func TestExtractImgURL(t *testing.T) {
	raw := "/images/search?pos=3&img_url=https%3A%2F%2Fexample.com%2Fphoto.jpg&rpt=simage"
	got := extractImgURL(raw)
	if got != "https://example.com/photo.jpg" {
		t.Errorf("extractImgURL = %q", got)
	}
	if extractImgURL("") != "" {
		t.Error("empty should return empty")
	}
}

type yandexStubDoer struct{ status int }

func (s *yandexStubDoer) Do(_, _ string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
	return nil, nil, s.status, nil
}

func TestYandexImages_httpFallbackToRenderer(t *testing.T) {
	y := &YandexImages{Renderer: nil}
	// HTTP returns 500, no renderer → error
	_, err := y.Search(context.Background(), &yandexStubDoer{status: 500}, "test", 10)
	if err == nil {
		t.Error("expected error when HTTP fails and renderer is nil")
	}
}

type yandexCaptchaDoer struct{}

func (d *yandexCaptchaDoer) Do(_, _ string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
	return []byte("<html></html>"), map[string]string{
		"x-yandex-captcha": "1",
	}, 200, nil
}

func TestYandexImages_detectsCaptcha(t *testing.T) {
	y := &YandexImages{}
	_, err := y.Search(context.Background(), &yandexCaptchaDoer{}, "test", 10)
	if err == nil {
		t.Fatal("expected captcha error")
	}
	if !strings.Contains(err.Error(), "captcha") {
		t.Errorf("error should mention captcha: %v", err)
	}
}

func TestParseYandexDataState_viewerData(t *testing.T) {
	html := `<div data-state="{&quot;form&quot;:{}}"></div>` +
		`<div data-state="{&quot;viewerData&quot;:{&quot;dups&quot;:[{&quot;url&quot;:&quot;https://img.com/a.jpg&quot;,&quot;fileSizeInBytes&quot;:12345,&quot;w&quot;:1024,&quot;h&quot;:768,&quot;title&quot;:&quot;Photo A&quot;,&quot;sourceName&quot;:&quot;example.com&quot;,&quot;sourceUrl&quot;:&quot;https://example.com/page&quot;,&quot;thumb&quot;:{&quot;url&quot;:&quot;//avatars.mds.yandex.net/i?id=aaa&quot;}}]}}"></div>`
	results := parseYandexDataState(html)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	r := results[0]
	if r.URL != "https://img.com/a.jpg" {
		t.Errorf("url = %q", r.URL)
	}
	if r.Width != 1024 || r.Height != 768 {
		t.Errorf("dims = %dx%d", r.Width, r.Height)
	}
	if r.Source != "https://example.com/page" {
		t.Errorf("source = %q", r.Source)
	}
	if r.Thumbnail != "https://avatars.mds.yandex.net/i?id=aaa" {
		t.Errorf("thumb = %q", r.Thumbnail)
	}
}
