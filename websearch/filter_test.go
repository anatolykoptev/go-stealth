package websearch

import "testing"

func TestFilterByScore_Basic(t *testing.T) {
	results := []Result{
		{Title: "A", URL: "http://a.com", Score: 0.9},
		{Title: "B", URL: "http://b.com", Score: 0.5},
		{Title: "C", URL: "http://c.com", Score: 0.3},
	}
	filtered := FilterByScore(results, 0.5, 1)
	if len(filtered) != 2 {
		t.Fatalf("got %d, want 2", len(filtered))
	}
}

func TestFilterByScore_KeepsMinKeep(t *testing.T) {
	results := []Result{
		{Title: "A", URL: "http://a.com", Score: 0.9},
		{Title: "B", URL: "http://b.com", Score: 0.1},
		{Title: "C", URL: "http://c.com", Score: 0.05},
	}
	filtered := FilterByScore(results, 0.8, 2)
	if len(filtered) != 2 {
		t.Fatalf("got %d, want 2 (minKeep)", len(filtered))
	}
}

func TestFilterByScore_AllBelowThreshold(t *testing.T) {
	results := []Result{
		{Title: "A", URL: "http://a.com", Score: 0.1},
		{Title: "B", URL: "http://b.com", Score: 0.2},
	}
	filtered := FilterByScore(results, 0.5, 5)
	// minKeep > len(results) -> return all
	if len(filtered) != 2 {
		t.Fatalf("got %d, want 2 (all returned when minKeep > len)", len(filtered))
	}
}

func TestDedupByDomain_Basic(t *testing.T) {
	results := []Result{
		{Title: "A1", URL: "http://example.com/a"},
		{Title: "A2", URL: "http://example.com/b"},
		{Title: "A3", URL: "http://example.com/c"},
		{Title: "B1", URL: "http://other.com/x"},
	}
	deduped := DedupByDomain(results, 2)
	if len(deduped) != 3 {
		t.Fatalf("got %d, want 3", len(deduped))
	}
}

func TestDedupByDomain_SinglePerDomain(t *testing.T) {
	results := []Result{
		{Title: "A1", URL: "http://example.com/a"},
		{Title: "A2", URL: "http://example.com/b"},
		{Title: "B1", URL: "http://other.com/x"},
		{Title: "B2", URL: "http://other.com/y"},
	}
	deduped := DedupByDomain(results, 1)
	if len(deduped) != 2 {
		t.Fatalf("got %d, want 2", len(deduped))
	}
}

func TestDedupByDomain_InvalidURL(t *testing.T) {
	results := []Result{
		{Title: "Good", URL: "http://example.com/a"},
		{Title: "Bad", URL: "://invalid"},
	}
	deduped := DedupByDomain(results, 10)
	if len(deduped) != 1 {
		t.Fatalf("got %d, want 1 (invalid URL skipped)", len(deduped))
	}
}
