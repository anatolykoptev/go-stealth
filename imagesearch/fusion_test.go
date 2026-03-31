package imagesearch

import "testing"

func TestFuseWRR_dedup(t *testing.T) {
	setA := []ImageResult{
		{URL: "https://a.com/1.jpg", Engine: "bing"},
		{URL: "https://a.com/2.jpg", Engine: "bing"},
	}
	setB := []ImageResult{
		{URL: "https://a.com/1.jpg", Engine: "ddg", Title: "DDG title"},
		{URL: "https://b.com/3.jpg", Engine: "ddg"},
	}
	fused := fuseWRR([][]ImageResult{setA, setB})
	if len(fused) != 3 {
		t.Fatalf("got %d results, want 3", len(fused))
	}
	if fused[0].URL != "https://a.com/1.jpg" {
		t.Errorf("first result URL = %q, want https://a.com/1.jpg", fused[0].URL)
	}
}

func TestFuseWRR_empty(t *testing.T) {
	if got := fuseWRR(nil); len(got) != 0 {
		t.Errorf("nil input: got %d, want 0", len(got))
	}
	if got := fuseWRR([][]ImageResult{{}}); len(got) != 0 {
		t.Errorf("empty set: got %d, want 0", len(got))
	}
}

func TestFuseWRR_single(t *testing.T) {
	set := []ImageResult{
		{URL: "https://a.jpg"}, {URL: "https://b.jpg"},
	}
	fused := fuseWRR([][]ImageResult{set})
	if len(fused) != 2 {
		t.Fatalf("got %d, want 2", len(fused))
	}
	if fused[0].URL != "https://a.jpg" {
		t.Errorf("first = %q, want https://a.jpg", fused[0].URL)
	}
}

func TestFuseWRR_skipsEmptyURL(t *testing.T) {
	set := []ImageResult{{URL: ""}, {URL: "https://real.jpg"}}
	fused := fuseWRR([][]ImageResult{set})
	if len(fused) != 1 {
		t.Fatalf("got %d, want 1", len(fused))
	}
}
