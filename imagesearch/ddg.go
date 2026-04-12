package imagesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
)

const (
	ddgImagesHome = "https://duckduckgo.com/"
	ddgImagesAPI  = "https://duckduckgo.com/i.js"
)

// ddgCookie provides consent cookies that prevent rate-limiting.
// SearXNG reference: searx/engines/duckduckgo.py
const ddgCookie = "ah=wt-wt; l=wt-wt; p=-1; ad=1"

// DdgImages searches DuckDuckGo Images via vqd token + /i.js endpoint.
type DdgImages struct{}

func (d *DdgImages) Name() string { return "ddg" }

func (d *DdgImages) Search(ctx context.Context, doer BrowserDoer, query string, max int) ([]ImageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Step 1: fetch homepage to get vqd token.
	tokenURL := ddgImagesHome + "?q=" + url.QueryEscape(query) + "&iax=images&ia=images"
	headers := searchHeaders()
	headers["referer"] = ddgImagesHome
	headers["cookie"] = ddgCookie

	data, _, status, err := doer.Do(http.MethodGet, tokenURL, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("ddg vqd request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("ddg vqd: status %d", status)
	}

	vqd := extractVQD(string(data))
	if vqd == "" {
		return nil, fmt.Errorf("ddg: vqd token not found")
	}

	// Step 2: fetch image results.
	imagesURL := fmt.Sprintf("%s?l=wt-wt&o=json&q=%s&vqd=%s&f=,,,,,&p=1",
		ddgImagesAPI, url.QueryEscape(query), url.QueryEscape(vqd))

	imgHeaders := searchHeaders()
	imgHeaders["referer"] = ddgImagesHome
	imgHeaders["x-requested-with"] = "XMLHttpRequest"
	imgHeaders["sec-fetch-site"] = "same-origin"
	imgHeaders["sec-fetch-mode"] = "cors"
	imgHeaders["cookie"] = ddgCookie

	imgData, _, imgStatus, err := doer.Do(http.MethodGet, imagesURL, imgHeaders, nil)
	if err != nil {
		return nil, fmt.Errorf("ddg images request: %w", err)
	}
	if imgStatus != http.StatusOK {
		return nil, fmt.Errorf("ddg images: status %d", imgStatus)
	}

	results := parseDDGImageJSON(imgData)
	if len(results) > max {
		results = results[:max]
	}
	return results, nil
}

type ddgImageResponse struct {
	Results []ddgImageResult `json:"results"`
}

type ddgImageResult struct {
	Image     string `json:"image"`
	Thumbnail string `json:"thumbnail"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

func parseDDGImageJSON(data []byte) []ImageResult {
	var resp ddgImageResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil
	}
	var results []ImageResult
	for _, r := range resp.Results {
		if r.Image == "" {
			continue
		}
		results = append(results, ImageResult{
			URL:       r.Image,
			Thumbnail: r.Thumbnail,
			Source:    r.URL,
			Title:     r.Title,
			Width:     r.Width,
			Height:    r.Height,
			Engine:    "ddg",
		})
	}
	return results
}

// vqdPatterns matches the DDG vqd token in various formats.
var vqdPatterns = []*regexp.Regexp{
	regexp.MustCompile(`vqd='([^']+)'`),
	regexp.MustCompile(`vqd="([^"]+)"`),
	regexp.MustCompile(`vqd=([a-zA-Z0-9_-]+)`),
}

// extractVQD extracts the vqd token from DDG HTML body.
func extractVQD(body string) string {
	for _, pat := range vqdPatterns {
		if m := pat.FindStringSubmatch(body); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}
