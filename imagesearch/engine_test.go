package imagesearch_test

import (
	"context"
	"io"
	"testing"

	"github.com/anatolykoptev/go-stealth/imagesearch"
)

type stubDoer struct{}

func (s *stubDoer) Do(method, url string, headers map[string]string, body io.Reader) ([]byte, map[string]string, int, error) {
	return nil, nil, 200, nil
}

type stubEngine struct{}

func (s *stubEngine) Search(_ context.Context, _ imagesearch.BrowserDoer, _ string, _ int) ([]imagesearch.ImageResult, error) {
	return nil, nil
}
func (s *stubEngine) Name() string { return "stub" }

type stubRenderer struct{}

func (s *stubRenderer) Render(_ context.Context, _ string) (string, error) {
	return "<html></html>", nil
}

func TestInterfaceSatisfaction(t *testing.T) {
	var _ imagesearch.BrowserDoer = (*stubDoer)(nil)
	var _ imagesearch.ImageEngine = (*stubEngine)(nil)
	var _ imagesearch.PageRenderer = (*stubRenderer)(nil)
}
