package imagesearch

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const braveImagesURL = "https://search.brave.com/images"

// braveCookie is required for Brave image search to return results.
const braveCookie = "safesearch=off; useLocation=0; summarizer=0; country=us; ui_lang=en-us"

// Brave uses JS object notation (unquoted keys), not JSON.
// properties:{url:"<img>",resized:"...",height:N,width:N,...}
var bravePropsRe = regexp.MustCompile(
	`properties:\{url:"([^"]+)",resized:"[^"]*",height:(\d+),width:(\d+)`,
)

// thumbnail:{src:"<thumb>"}
var braveThumbRe = regexp.MustCompile(`thumbnail:\{src:"([^"]+)"`)

// BraveImages searches Brave via embedded JS objects in SSR HTML.
type BraveImages struct{}

func (b *BraveImages) Name() string { return "brave" }

func (b *BraveImages) Search(ctx context.Context, doer BrowserDoer, query string, max int) ([]ImageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	u := braveImagesURL + "?q=" + url.QueryEscape(query) + "&source=web"
	headers := searchHeaders()
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

// parseBraveImageHTML extracts image results from Brave SSR HTML.
// Brave uses JS object notation: properties:{url:"...",height:N,width:N}
// Each result block has properties block + nearby thumbnail:{src:"..."}.
func parseBraveImageHTML(html string) []ImageResult {
	propLocs := bravePropsRe.FindAllStringSubmatchIndex(html, -1)
	if len(propLocs) == 0 {
		return nil
	}

	var results []ImageResult
	for _, loc := range propLocs {
		imgURL := html[loc[2]:loc[3]]
		h, _ := strconv.Atoi(html[loc[4]:loc[5]])
		w, _ := strconv.Atoi(html[loc[6]:loc[7]])

		// Look backwards for source URL: {url:"<source>",title:"<title>"
		start := loc[0] - 500
		if start < 0 {
			start = 0
		}
		prefix := html[start:loc[0]]

		source := extractLastQuoted(prefix, `{url:"`)
		title := extractLastQuoted(prefix, `title:"`)

		// Look forward for thumbnail
		end := loc[1] + 300
		if end > len(html) {
			end = len(html)
		}
		thumb := ""
		if tm := braveThumbRe.FindStringSubmatch(html[loc[1]:end]); len(tm) > 1 {
			thumb = tm[1]
		}

		results = append(results, ImageResult{
			URL:       imgURL,
			Source:    source,
			Title:     title,
			Thumbnail: thumb,
			Width:     w,
			Height:    h,
			Engine:    "brave",
		})
	}
	return results
}

// extractLastQuoted finds the last occurrence of prefix+"<value>" and returns value.
func extractLastQuoted(s, prefix string) string {
	idx := strings.LastIndex(s, prefix)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}
