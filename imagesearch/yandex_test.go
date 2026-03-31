package imagesearch

import "testing"

func TestParseYandexDataState_extracts(t *testing.T) {
	html := `<div data-state="{&quot;initialState&quot;:{&quot;serpList&quot;:{&quot;items&quot;:{&quot;entities&quot;:{&quot;0&quot;:{&quot;origUrl&quot;:&quot;https://img.com/photo.jpg&quot;,&quot;thumbUrl&quot;:&quot;https://im0-tub-ru.yandex.net/1.jpg&quot;,&quot;title&quot;:&quot;Photo&quot;,&quot;width&quot;:1200,&quot;height&quot;:800,&quot;sourceUrl&quot;:&quot;https://example.com&quot;},&quot;1&quot;:{&quot;origUrl&quot;:&quot;https://img.com/cat.jpg&quot;,&quot;thumbUrl&quot;:&quot;https://im0-tub-ru.yandex.net/2.jpg&quot;,&quot;title&quot;:&quot;Cat&quot;,&quot;width&quot;:800,&quot;height&quot;:600,&quot;sourceUrl&quot;:&quot;https://cats.com&quot;}}}}}}" ></div>`
	results := parseYandexDataState(html)
	if len(results) != 2 {
		t.Fatalf("got %d, want 2", len(results))
	}
	if results[0].Engine != "yandex" {
		t.Errorf("engine = %q", results[0].Engine)
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

func TestParseYandexDataState_nilRenderer(t *testing.T) {
	y := &YandexImages{Renderer: nil}
	results, err := y.Search(nil, nil, "test", 10)
	if err != nil {
		t.Errorf("nil renderer should not error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("nil renderer should return empty, got %d", len(results))
	}
}
