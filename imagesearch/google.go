package imagesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const googleImagesURL = "https://www.google.com/search"

// GoogleImages searches Google Images via the Android UA JSON endpoint.
// Google returns JSON (ischj format) when it sees an Android/Dalvik UA.
// Falls back to PageRenderer (Chrome) if HTTP returns non-200 or 0 results.
// Reference: SearXNG searx/engines/google_images.py
type GoogleImages struct {
	Renderer PageRenderer // optional Chrome fallback
}

func (g *GoogleImages) Name() string { return "google" }

func (g *GoogleImages) Search(ctx context.Context, doer BrowserDoer, query string, max int) ([]ImageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// SearXNG: async param must NOT be url-encoded, asearch=isch (not ischab).
	u := fmt.Sprintf("%s?q=%s&tbm=isch&asearch=isch&async=_fmt:json,p:1,ijn:0",
		googleImagesURL, url.QueryEscape(query))

	headers := androidHeaders()

	data, _, status, err := doer.Do(http.MethodGet, u, headers, nil)
	if err == nil && status == http.StatusOK {
		if results := parseGoogleImageJSON(data); len(results) > 0 {
			if len(results) > max {
				results = results[:max]
			}
			return results, nil
		}
	}

	// Fallback to Chrome render if available.
	if g.Renderer == nil {
		if err != nil {
			return nil, fmt.Errorf("google images request: %w", err)
		}
		return nil, fmt.Errorf("google images: status %d, no renderer", status)
	}
	renderedHTML, err := g.Renderer.Render(ctx, "https://www.google.com/search?q="+url.QueryEscape(query)+"&udm=2")
	if err != nil {
		return nil, fmt.Errorf("google render: %w", err)
	}
	results := parseGoogleImageJSON([]byte(renderedHTML))
	if len(results) > max {
		results = results[:max]
	}
	return results, nil
}

type googleIschj struct {
	Ischj struct {
		Metadata []googleMetadata `json:"metadata"`
	} `json:"ischj"`
}

type googleMetadata struct {
	Result        googleResult `json:"result"`
	OriginalImage googleImage  `json:"original_image"`
	Thumbnail     googleThumb  `json:"thumbnail"`
}

type googleResult struct {
	ReferrerURL string `json:"referrer_url"`
	PageTitle   string `json:"page_title"`
}

type googleImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type googleThumb struct {
	URL string `json:"url"`
}

// parseGoogleImageJSON parses Google's ischj JSON response.
// The JSON is embedded in a larger response; find {"ischj": marker first.
// Reference: SearXNG uses resp.text.find('{"ischj":').
func parseGoogleImageJSON(data []byte) []ImageResult {
	marker := []byte(`{"ischj":`)
	idx := bytes.Index(data, marker)
	if idx < 0 {
		return nil
	}
	data = data[idx:]

	var resp googleIschj
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil
	}

	var results []ImageResult
	for _, m := range resp.Ischj.Metadata {
		if m.OriginalImage.URL == "" {
			continue
		}
		results = append(results, ImageResult{
			URL:       m.OriginalImage.URL,
			Thumbnail: m.Thumbnail.URL,
			Source:    m.Result.ReferrerURL,
			Title:     m.Result.PageTitle,
			Width:     m.OriginalImage.Width,
			Height:    m.OriginalImage.Height,
			Engine:    "google",
		})
	}
	return results
}
