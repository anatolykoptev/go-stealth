package imagesearch

import (
	"context"
	"io"
	"testing"
)

type fakeDoer struct{}

func (f *fakeDoer) Do(_, _ string, _ map[string]string, _ io.Reader) ([]byte, map[string]string, int, error) {
	return nil, nil, 200, nil
}

type fakeEngine struct {
	name    string
	results []ImageResult
	err     error
}

func (f *fakeEngine) Name() string { return f.name }
func (f *fakeEngine) Search(_ context.Context, _ BrowserDoer, _ string, _ int) ([]ImageResult, error) {
	return f.results, f.err
}

func TestMultiSearch_mergesAndFuses(t *testing.T) {
	ms := &MultiSearch{
		Engines: []ImageEngine{
			&fakeEngine{name: "a", results: []ImageResult{
				{URL: "https://shared.jpg", Engine: "a"},
				{URL: "https://only-a.jpg", Engine: "a"},
			}},
			&fakeEngine{name: "b", results: []ImageResult{
				{URL: "https://shared.jpg", Engine: "b"},
				{URL: "https://only-b.jpg", Engine: "b"},
			}},
		},
		Doer: &fakeDoer{},
	}
	results := ms.Search(context.Background(), "test", 10)
	if len(results) != 3 {
		t.Fatalf("got %d, want 3", len(results))
	}
	if results[0].URL != "https://shared.jpg" {
		t.Errorf("first = %q, want https://shared.jpg", results[0].URL)
	}
}

func TestMultiSearch_truncates(t *testing.T) {
	ms := &MultiSearch{
		Engines: []ImageEngine{
			&fakeEngine{name: "a", results: []ImageResult{
				{URL: "https://1.jpg"}, {URL: "https://2.jpg"}, {URL: "https://3.jpg"},
			}},
		},
		Doer: &fakeDoer{},
	}
	results := ms.Search(context.Background(), "test", 2)
	if len(results) != 2 {
		t.Fatalf("got %d, want 2", len(results))
	}
}

func TestMultiSearch_noEngines(t *testing.T) {
	ms := &MultiSearch{Doer: &fakeDoer{}}
	results := ms.Search(context.Background(), "test", 10)
	if len(results) != 0 {
		t.Errorf("no engines: got %d", len(results))
	}
}

func TestMultiSearch_engineErrorIsNonFatal(t *testing.T) {
	ms := &MultiSearch{
		Engines: []ImageEngine{
			&fakeEngine{name: "ok", results: []ImageResult{{URL: "https://ok.jpg"}}},
			&fakeEngine{name: "fail", err: context.DeadlineExceeded},
		},
		Doer: &fakeDoer{},
	}
	results := ms.Search(context.Background(), "test", 10)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
}
