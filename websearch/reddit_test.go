package websearch

import (
	"context"
	"io"
	"net/http"
	"testing"
)

const redditFixture = `{
	"data": {
		"children": [
			{
				"data": {
					"title": "Why Go is great",
					"permalink": "/r/golang/comments/abc123/why_go_is_great/",
					"selftext": "Go has excellent concurrency support.",
					"score": 42,
					"num_comments": 10,
					"subreddit": "golang",
					"url": "https://www.reddit.com/r/golang/comments/abc123/why_go_is_great/"
				}
			},
			{
				"data": {
					"title": "Rust vs Go performance",
					"permalink": "/r/programming/comments/def456/rust_vs_go/",
					"selftext": "",
					"score": 128,
					"num_comments": 55,
					"subreddit": "programming",
					"url": "https://example.com/blog/rust-vs-go"
				}
			}
		]
	}
}`

func TestParseRedditJSON(t *testing.T) {
	results, err := ParseRedditJSON([]byte(redditFixture))
	if err != nil {
		t.Fatalf("ParseRedditJSON: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	r := results[0]
	if r.Title != "Why Go is great" {
		t.Errorf("Title = %q, want %q", r.Title, "Why Go is great")
	}
	if r.URL != "https://www.reddit.com/r/golang/comments/abc123/why_go_is_great/" {
		t.Errorf("URL = %q", r.URL)
	}
	if r.Score != directResultScore {
		t.Errorf("Score = %f, want %f", r.Score, directResultScore)
	}
	if r.Metadata["engine"] != "reddit" {
		t.Errorf("Metadata[engine] = %q, want reddit", r.Metadata["engine"])
	}
	if r.Metadata["subreddit"] != "golang" {
		t.Errorf("Metadata[subreddit] = %q, want golang", r.Metadata["subreddit"])
	}

	r2 := results[1]
	if r2.Metadata["subreddit"] != "programming" {
		t.Errorf("results[1].Metadata[subreddit] = %q, want programming", r2.Metadata["subreddit"])
	}
}

func TestReddit_SearchMock(t *testing.T) {
	bc := &mockBrowser{fn: func(method, u string, h map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
		if method != http.MethodGet {
			t.Errorf("method = %q, want GET", method)
		}
		if h["accept"] != "application/json" {
			t.Errorf("accept header = %q, want application/json", h["accept"])
		}
		return []byte(redditFixture), nil, http.StatusOK, nil
	}}

	r := NewReddit(WithRedditBrowser(bc))
	results, err := r.Search(context.Background(), "golang", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Title != "Why Go is great" {
		t.Errorf("Title = %q", results[0].Title)
	}
}

func TestReddit_RateLimitStatus(t *testing.T) {
	bc := &mockBrowser{fn: func(_, _ string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
		return []byte("error"), nil, http.StatusTooManyRequests, nil
	}}

	r := NewReddit(WithRedditBrowser(bc))
	_, err := r.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Fatal("expected error on 429 status")
	}
	rl, ok := err.(*ErrRateLimited)
	if !ok {
		t.Fatalf("expected ErrRateLimited, got %T: %v", err, err)
	}
	if rl.Engine != "reddit" {
		t.Errorf("Engine = %q, want reddit", rl.Engine)
	}
}

func TestReddit_RateLimitJSON(t *testing.T) {
	body := `{"error": 429, "message": "Too Many Requests"}`
	bc := &mockBrowser{fn: func(_, _ string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
		return []byte(body), nil, http.StatusOK, nil
	}}

	r := NewReddit(WithRedditBrowser(bc))
	_, err := r.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Fatal("expected error on rate-limit JSON body")
	}
	rl, ok := err.(*ErrRateLimited)
	if !ok {
		t.Fatalf("expected ErrRateLimited, got %T: %v", err, err)
	}
	if rl.Engine != "reddit" {
		t.Errorf("Engine = %q, want reddit", rl.Engine)
	}
}

func TestReddit_NoBrowser(t *testing.T) {
	r := NewReddit()
	_, err := r.Search(context.Background(), "test", SearchOpts{})
	if err == nil {
		t.Fatal("expected error when no BrowserDoer provided")
	}
}
