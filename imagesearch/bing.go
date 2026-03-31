package imagesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	bingAsyncURL = "https://www.bing.com/images/async"
	bingMaxCount = 35
)

var bingMAttrRe = regexp.MustCompile(`m="(\{[^"]*\})"`)

// BingImages searches Bing via the /images/async AJAX endpoint.
type BingImages struct{}

func (b *BingImages) Name() string { return "bing" }

func (b *BingImages) Search(ctx context.Context, doer BrowserDoer, query string, max int) ([]ImageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	count := max
	if count > bingMaxCount {
		count = bingMaxCount
	}

	u := fmt.Sprintf("%s?q=%s&first=0&count=%d&mmasync=1",
		bingAsyncURL, url.QueryEscape(query), count)

	headers := searchHeaders()
	headers["accept-language"] = "en-US,en;q=0.9"

	data, _, status, err := doer.Do(http.MethodGet, u, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("bing request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("bing: status %d", status)
	}

	results := parseBingHTML(string(data))
	if len(results) > max {
		results = results[:max]
	}
	return results, nil
}

type bingMAttr struct {
	Murl string `json:"murl"`
	Turl string `json:"turl"`
	Purl string `json:"purl"`
	T    string `json:"t"`
	Mw   string `json:"mw"`
	Mh   string `json:"mh"`
}

func parseBingHTML(html string) []ImageResult {
	var results []ImageResult
	for _, cap := range bingMAttrRe.FindAllStringSubmatch(html, -1) {
		raw := cap[1]
		jsonStr := strings.ReplaceAll(raw, "&quot;", `"`)
		jsonStr = strings.ReplaceAll(jsonStr, "&amp;", "&")
		var attr bingMAttr
		if err := json.Unmarshal([]byte(jsonStr), &attr); err != nil {
			continue
		}
		if attr.Murl == "" {
			continue
		}
		w, _ := strconv.Atoi(attr.Mw)
		h, _ := strconv.Atoi(attr.Mh)
		results = append(results, ImageResult{
			URL:       attr.Murl,
			Thumbnail: attr.Turl,
			Source:    attr.Purl,
			Title:     attr.T,
			Width:     w,
			Height:    h,
			Engine:    "bing",
		})
	}
	return results
}
