package websearch

import (
	"context"
	"errors"
	"testing"
)

type failProvider struct {
	err error
}

func (f *failProvider) Search(_ context.Context, _ string, _ SearchOpts) ([]Result, error) {
	return nil, f.err
}

type okProvider struct {
	name string
}

func (o *okProvider) Search(_ context.Context, _ string, _ SearchOpts) ([]Result, error) {
	return []Result{{Title: o.name, URL: "http://" + o.name + ".com"}}, nil
}

func TestFallback_PrimarySucceeds(t *testing.T) {
	t.Parallel()
	fb := NewFallback(&okProvider{"primary"}, &okProvider{"secondary"})
	results, err := fb.Search(context.Background(), "q", SearchOpts{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if results[0].Title != "primary" {
		t.Errorf("expected primary, got %q", results[0].Title)
	}
}

func TestFallback_PrimaryFails(t *testing.T) {
	t.Parallel()
	fb := NewFallback(
		&failProvider{errors.New("primary down")},
		&okProvider{"secondary"},
	)
	results, err := fb.Search(context.Background(), "q", SearchOpts{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if results[0].Title != "secondary" {
		t.Errorf("expected secondary, got %q", results[0].Title)
	}
}

func TestFallback_AllFail(t *testing.T) {
	t.Parallel()
	fb := NewFallback(
		&failProvider{errors.New("first down")},
		&failProvider{errors.New("second down")},
	)
	_, err := fb.Search(context.Background(), "q", SearchOpts{})
	if err == nil {
		t.Error("expected error when all providers fail")
	}
}

func TestFallback_ThreeProviders(t *testing.T) {
	t.Parallel()
	fb := NewFallback(
		&failProvider{errors.New("first down")},
		&failProvider{errors.New("second down")},
		&okProvider{"third"},
	)
	results, err := fb.Search(context.Background(), "q", SearchOpts{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if results[0].Title != "third" {
		t.Errorf("expected third, got %q", results[0].Title)
	}
}

func TestFallback_SingleProvider(t *testing.T) {
	t.Parallel()
	fb := NewFallback(&okProvider{"only"})
	results, err := fb.Search(context.Background(), "q", SearchOpts{})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if results[0].Title != "only" {
		t.Errorf("expected 'only', got %q", results[0].Title)
	}
}
