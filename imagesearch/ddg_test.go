package imagesearch

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestParseDDGImageJSON(t *testing.T) {
	body := `{"results":[{"image":"https://img.com/a.jpg","thumbnail":"https://th.com/a.jpg","url":"https://page.com/a","title":"Cat photo","width":800,"height":600},{"image":"https://img.com/b.jpg","thumbnail":"https://th.com/b.jpg","url":"https://page.com/b","title":"Dog photo","width":1024,"height":768}]}`
	results := parseDDGImageJSON([]byte(body))
	if len(results) != 2 {
		t.Fatalf("got %d, want 2", len(results))
	}
	if results[0].URL != "https://img.com/a.jpg" {
		t.Errorf("url[0] = %q", results[0].URL)
	}
	if results[0].Width != 800 {
		t.Errorf("width[0] = %d, want 800", results[0].Width)
	}
	if results[0].Engine != "ddg" {
		t.Errorf("engine[0] = %q", results[0].Engine)
	}
}

func TestParseDDGImageJSON_empty(t *testing.T) {
	if got := parseDDGImageJSON([]byte(`{}`)); len(got) != 0 {
		t.Errorf("empty obj: got %d", len(got))
	}
	if got := parseDDGImageJSON([]byte(`{"results":[]}`)); len(got) != 0 {
		t.Errorf("empty arr: got %d", len(got))
	}
}

func TestParseDDGImageJSON_filterEmptyImage(t *testing.T) {
	body := `{"results":[{"image":"","thumbnail":"t.jpg","url":"p.com","title":"No img","width":0,"height":0},{"image":"https://real.jpg","thumbnail":"t2.jpg","url":"p2.com","title":"Real","width":100,"height":100}]}`
	results := parseDDGImageJSON([]byte(body))
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].URL != "https://real.jpg" {
		t.Errorf("url = %q", results[0].URL)
	}
}

type ddgHeaderCapture struct {
	vqdHeaders map[string]string
	imgHeaders map[string]string
	callCount  int
}

func (d *ddgHeaderCapture) Do(method, rawURL string, headers map[string]string, body io.Reader) ([]byte, map[string]string, int, error) {
	d.callCount++
	if d.callCount == 1 {
		d.vqdHeaders = copyMap(headers)
		return []byte(`<script>vqd="4-123456"</script>`), nil, 200, nil
	}
	d.imgHeaders = copyMap(headers)
	return []byte(`{"results":[]}`), nil, 200, nil
}

func copyMap(m map[string]string) map[string]string {
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func TestDdgImages_sendsCookies(t *testing.T) {
	cap := &ddgHeaderCapture{}
	d := &DdgImages{}
	_, _ = d.Search(context.Background(), cap, "test", 10)

	if cap.imgHeaders["cookie"] == "" {
		t.Error("cookie header missing on image request")
	}
	for _, key := range []string{"ah", "l"} {
		if !strings.Contains(cap.imgHeaders["cookie"], key+"=") {
			t.Errorf("missing cookie key %q in %q", key, cap.imgHeaders["cookie"])
		}
	}
}
