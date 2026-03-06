package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearXNG_Search(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("expected /search path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "json" {
			t.Error("expected format=json")
		}
		if r.Header.Get("X-Forwarded-For") != "127.0.0.1" {
			t.Error("expected X-Forwarded-For: 127.0.0.1")
		}
		resp := searxngResponse{
			Results: []searxngRawResult{
				{URL: "https://example.com/1", Title: "Result 1", Content: "Content 1", Score: 0.9},
				{URL: "https://example.com/2", Title: "Result 2", Content: "Content 2", Score: 0.8},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewSearXNG(srv.URL)
	results, err := s.Search(context.Background(), "test query", SearchOpts{})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Result 1" {
		t.Errorf("expected title 'Result 1', got %q", results[0].Title)
	}
}

func TestSearXNG_SearchAdvanced(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("language") != "ru" {
			t.Errorf("expected language=ru, got %q", q.Get("language"))
		}
		if q.Get("time_range") != "week" {
			t.Errorf("expected time_range=week, got %q", q.Get("time_range"))
		}
		if q.Get("engines") != "google,bing" {
			t.Errorf("expected engines=google,bing, got %q", q.Get("engines"))
		}
		if q.Get("categories") != "general" {
			t.Errorf("expected categories=general, got %q", q.Get("categories"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searxngResponse{})
	}))
	defer srv.Close()

	s := NewSearXNG(srv.URL)
	_, err := s.SearchAdvanced(context.Background(), "test", "ru", "week", "google,bing", "general")
	if err != nil {
		t.Fatalf("SearchAdvanced error: %v", err)
	}
}

func TestSearXNG_MetadataHandling(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		raw := `{"results":[
			{"url":"https://a.com","title":"A","content":"CA","metadata":{"key":"val"}},
			{"url":"https://b.com","title":"B","content":"CB","metadata":""},
			{"url":"https://c.com","title":"C","content":"CC"}
		]}`
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer srv.Close()

	s := NewSearXNG(srv.URL)
	results, err := s.Search(context.Background(), "test", SearchOpts{})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Metadata["key"] != "val" {
		t.Errorf("expected metadata key=val, got %v", results[0].Metadata)
	}
	if results[1].Metadata != nil {
		t.Errorf("expected nil metadata for empty string, got %v", results[1].Metadata)
	}
}

func TestSearXNG_MaxResults(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := searxngResponse{
			Results: []searxngRawResult{
				{URL: "https://a.com", Title: "A"},
				{URL: "https://b.com", Title: "B"},
				{URL: "https://c.com", Title: "C"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewSearXNG(srv.URL, WithSearXNGMaxResults(2))
	results, err := s.Search(context.Background(), "test", SearchOpts{})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results with max=2, got %d", len(results))
	}
}

func TestSearXNG_EmptyResults(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searxngResponse{})
	}))
	defer srv.Close()

	s := NewSearXNG(srv.URL)
	results, err := s.Search(context.Background(), "test", SearchOpts{})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearXNG_ServerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := NewSearXNG(srv.URL)
	_, err := s.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestSearXNG_LanguageAll(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("language") {
			t.Error("language should not be sent when set to 'all'")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searxngResponse{})
	}))
	defer srv.Close()

	s := NewSearXNG(srv.URL)
	_, _ = s.Search(context.Background(), "test", SearchOpts{Language: "all"})
}
