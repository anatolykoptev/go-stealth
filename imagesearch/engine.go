package imagesearch

import (
	"context"
	"io"
)

// BrowserDoer performs HTTP requests with browser-like TLS fingerprint.
// Identical to websearch.BrowserDoer — type alias to avoid circular import.
type BrowserDoer interface {
	Do(method, url string, headers map[string]string, body io.Reader) ([]byte, map[string]string, int, error)
}

// ImageResult holds a single image search result.
type ImageResult struct {
	URL       string // direct image URL
	Thumbnail string // thumbnail URL
	Source    string // page URL where image was found
	Title     string // image or page title
	Width     int    // image width (0 if unknown)
	Height    int    // image height (0 if unknown)
	Engine    string // engine name that found this result
}

// ImageEngine searches for images from a single source.
type ImageEngine interface {
	Search(ctx context.Context, doer BrowserDoer, query string, max int) ([]ImageResult, error)
	Name() string
}

// PageRenderer renders a URL in a real browser and returns full HTML.
// Implemented by go-browser; injected by the consumer via DI.
// Optional — engines that need it (Yandex) skip gracefully when nil.
type PageRenderer interface {
	Render(ctx context.Context, url string) (html string, err error)
}
