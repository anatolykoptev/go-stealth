package imagesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	stealth "github.com/anatolykoptev/go-stealth"
)

const braveImagesURL = "https://search.brave.com/images"

// braveCookie is required for Brave image search to return results.
const braveCookie = "safesearch=off; useLocation=0; summarizer=0; country=us; ui_lang=en-us"

var braveResultRe = regexp.MustCompile(
	`\{"url":"[^"]+","title":"[^"]*"[^}]*"properties":\{[^}]+\}[^}]*"thumbnail":\{[^}]+\}\}`,
)

// BraveImages searches Brave via embedded SvelteKit JSON in HTML.
type BraveImages struct{}

func (b *BraveImages) Name() string { return "brave" }

func (b *BraveImages) Search(ctx context.Context, doer BrowserDoer, query string, max int) ([]ImageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	u := braveImagesURL + "?q=" + url.QueryEscape(query) + "&source=web"
	headers := stealth.ChromeHeaders()
	headers["cookie"] = braveCookie

	data, _, status, err := doer.Do(http.MethodGet, u, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("brave images request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("brave images: status %d", status)
	}

	results := parseBraveImageHTML(string(data))
	if len(results) > max {
		results = results[:max]
	}
	return results, nil
}

type braveResult struct {
	URL        string           `json:"url"`
	Title      string           `json:"title"`
	Properties *braveProperties `json:"properties"`
	Thumbnail  *braveThumbnail  `json:"thumbnail"`
}

type braveProperties struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type braveThumbnail struct {
	Src string `json:"src"`
}

func parseBraveImageHTML(html string) []ImageResult {
	matches := braveResultRe.FindAllString(html, -1)
	var results []ImageResult
	for _, m := range matches {
		var br braveResult
		if err := json.Unmarshal([]byte(m), &br); err != nil {
			continue
		}
		if br.Properties == nil || br.Properties.URL == "" {
			continue
		}
		r := ImageResult{
			URL:    br.Properties.URL,
			Source: br.URL,
			Title:  br.Title,
			Width:  br.Properties.Width,
			Height: br.Properties.Height,
			Engine: "brave",
		}
		if br.Thumbnail != nil {
			r.Thumbnail = br.Thumbnail.Src
		}
		results = append(results, r)
	}
	return results
}
