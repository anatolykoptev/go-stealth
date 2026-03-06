package websearch

import "testing"

func TestFuseWRR_TwoSources(t *testing.T) {
	set1 := []Result{
		{Title: "A", URL: "http://a.com", Score: 1.0},
		{Title: "B", URL: "http://b.com", Score: 1.0},
	}
	set2 := []Result{
		{Title: "B (dup)", URL: "http://b.com", Score: 1.0},
		{Title: "C", URL: "http://c.com", Score: 1.0},
	}

	merged := FuseWRR([][]Result{set1, set2}, []float64{1.0, 1.0})

	// B appears in both -> highest fused score.
	if len(merged) != 3 {
		t.Fatalf("got %d results, want 3", len(merged))
	}
	if merged[0].URL != "http://b.com" {
		t.Errorf("top result URL = %q, want http://b.com (appears in both)", merged[0].URL)
	}
}

func TestFuseWRR_EmptyInput(t *testing.T) {
	merged := FuseWRR(nil, nil)
	if merged != nil {
		t.Errorf("expected nil for empty input, got %v", merged)
	}
}

func TestFuseWRR_SingleSource(t *testing.T) {
	set := []Result{
		{Title: "A", URL: "http://a.com"},
		{Title: "B", URL: "http://b.com"},
	}
	merged := FuseWRR([][]Result{set}, []float64{1.0})
	if len(merged) != 2 {
		t.Fatalf("got %d, want 2", len(merged))
	}
	if merged[0].URL != "http://a.com" {
		t.Errorf("first = %q, want http://a.com", merged[0].URL)
	}
}

func TestFuseWRR_WeightsAffectRanking(t *testing.T) {
	set1 := []Result{{Title: "A", URL: "http://a.com"}}
	set2 := []Result{{Title: "B", URL: "http://b.com"}}

	merged := FuseWRR([][]Result{set1, set2}, []float64{2.0, 1.0})
	if len(merged) != 2 {
		t.Fatalf("got %d, want 2", len(merged))
	}
	if merged[0].URL != "http://a.com" {
		t.Errorf("first = %q, want http://a.com (higher weight)", merged[0].URL)
	}
}

func TestFuseWRR_DuplicateURL_MergesMetadata(t *testing.T) {
	set1 := []Result{{Title: "A from DDG", URL: "http://a.com", Content: "ddg snippet"}}
	set2 := []Result{{Title: "A from SP", URL: "http://a.com", Content: "sp snippet"}}

	merged := FuseWRR([][]Result{set1, set2}, []float64{1.0, 1.0})
	if len(merged) != 1 {
		t.Fatalf("got %d, want 1 (deduped by URL)", len(merged))
	}
	if merged[0].Title != "A from DDG" {
		t.Errorf("title = %q, want 'A from DDG' (first seen)", merged[0].Title)
	}
}
