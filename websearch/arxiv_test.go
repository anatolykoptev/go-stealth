package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const arxivFixture = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2301.00001v1</id>
    <title>Attention Is All You Need</title>
    <summary>We propose a new architecture based on attention mechanisms.</summary>
    <author><name>Alice Smith</name></author>
    <author><name>Bob Jones</name></author>
    <published>2023-01-01T00:00:00Z</published>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2301.00002v1</id>
    <title>Deep Residual Learning</title>
    <summary>Residual connections help train very deep networks.</summary>
    <author><name>Carol Lee</name></author>
    <published>2023-01-02T00:00:00Z</published>
  </entry>
</feed>`

func TestParseArxivAtom(t *testing.T) {
	results, err := ParseArxivAtom([]byte(arxivFixture))
	if err != nil {
		t.Fatalf("ParseArxivAtom: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	r := results[0]
	if r.Title != "Attention Is All You Need" {
		t.Errorf("Title = %q, want Attention Is All You Need", r.Title)
	}
	if r.URL != "http://arxiv.org/abs/2301.00001v1" {
		t.Errorf("URL = %q", r.URL)
	}
	if r.Score != directResultScore {
		t.Errorf("Score = %f, want %f", r.Score, directResultScore)
	}
	if r.Metadata["engine"] != "arxiv" {
		t.Errorf("Metadata[engine] = %q, want arxiv", r.Metadata["engine"])
	}
	if want := "Authors: Alice Smith, Bob Jones\n"; len(r.Content) < len(want) {
		t.Errorf("Content too short: %q", r.Content)
	}

	r2 := results[1]
	if r2.Title != "Deep Residual Learning" {
		t.Errorf("results[1].Title = %q", r2.Title)
	}
}

func TestArxiv_SearchMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("search_query")
		if q == "" {
			t.Error("missing search_query param")
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(arxivFixture))
	}))
	defer srv.Close()

	a := NewArxiv(WithArxivBaseURL(srv.URL))
	results, err := a.Search(context.Background(), "transformers", SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Title != "Attention Is All You Need" {
		t.Errorf("Title = %q", results[0].Title)
	}
}

func TestArxiv_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := NewArxiv(WithArxivBaseURL(srv.URL))
	_, err := a.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Fatal("expected error on 500 status")
	}
}
