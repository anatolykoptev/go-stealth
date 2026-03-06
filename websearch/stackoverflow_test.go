package websearch

import (
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const soFixture = `{
  "items": [
    {
      "title": "How to parse JSON in Go?",
      "link": "https://stackoverflow.com/questions/111/how-to-parse-json",
      "score": 42,
      "answer_count": 5,
      "tags": ["go", "json"],
      "is_answered": true
    },
    {
      "title": "Go error handling best practices",
      "link": "https://stackoverflow.com/questions/222/go-error-handling",
      "score": 18,
      "answer_count": 3,
      "tags": ["go", "error-handling"],
      "is_answered": false
    }
  ],
  "has_more": true,
  "quota_remaining": 298
}`

func TestParseStackOverflowJSON(t *testing.T) {
	results, err := ParseStackOverflowJSON([]byte(soFixture))
	if err != nil {
		t.Fatalf("ParseStackOverflowJSON: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	r := results[0]
	if r.Title != "How to parse JSON in Go?" {
		t.Errorf("Title = %q", r.Title)
	}
	if r.URL != "https://stackoverflow.com/questions/111/how-to-parse-json" {
		t.Errorf("URL = %q", r.URL)
	}
	if r.Score != directResultScore {
		t.Errorf("Score = %f, want %f", r.Score, directResultScore)
	}
	if r.Metadata["engine"] != "stackoverflow" {
		t.Errorf("Metadata[engine] = %q", r.Metadata["engine"])
	}
	if want := "[go, json] | 42 pts | 5 answers | answered"; r.Content != want {
		t.Errorf("Content = %q, want %q", r.Content, want)
	}

	r2 := results[1]
	if want := "[go, error-handling] | 18 pts | 3 answers | unanswered"; r2.Content != want {
		t.Errorf("Content = %q, want %q", r2.Content, want)
	}
}

func TestStackOverflow_SearchMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			t.Error("missing q param")
		}
		if r.URL.Query().Get("site") != "stackoverflow" {
			t.Error("missing site=stackoverflow param")
		}
		// SE API always returns gzip
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(soFixture))
		gz.Close()
	}))
	defer srv.Close()

	so := NewStackOverflow(WithSOBaseURL(srv.URL))
	results, err := so.Search(context.Background(), "parse json go", SearchOpts{Limit: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Title != "How to parse JSON in Go?" {
		t.Errorf("Title = %q", results[0].Title)
	}
}

func TestStackOverflow_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	so := NewStackOverflow(WithSOBaseURL(srv.URL))
	_, err := so.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Fatal("expected error on 500 status")
	}
}

func TestSOCheckLimit(t *testing.T) {
	// Reset counter for deterministic test.
	soDaily.count.Store(0)
	soDaily.day.Store(0)

	// First call should succeed.
	if !soCheckLimit() {
		t.Fatal("first call should be under limit")
	}

	// Exhaust remaining quota.
	soDaily.count.Store(soDailyLimit)

	// Next call should exceed limit.
	if soCheckLimit() {
		t.Fatal("should be over limit after 300 requests")
	}
}
