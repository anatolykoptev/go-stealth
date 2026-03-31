package imagesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const yandexImagesURL = "https://yandex.ru/images/search"

var dataStateRe = regexp.MustCompile(`data-state="([^"]+)"`)

// YandexImages searches Yandex Images via plain HTTP + data-state JSON parsing.
// Falls back to PageRenderer (Chrome) if doer returns empty results.
type YandexImages struct {
	Renderer PageRenderer // optional Chrome fallback
}

func (y *YandexImages) Name() string { return "yandex" }

func (y *YandexImages) Search(ctx context.Context, doer BrowserDoer, query string, max int) ([]ImageResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	u := yandexImagesURL + "?text=" + url.QueryEscape(query)

	// Try plain HTTP first (works without Chrome).
	headers := searchHeaders()
	headers["accept-language"] = "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7"

	data, respHeaders, status, err := doer.Do(http.MethodGet, u, headers, nil)
	if err == nil && status == http.StatusOK {
		if respHeaders["x-yandex-captcha"] != "" {
			if y.Renderer == nil {
				return nil, fmt.Errorf("yandex: captcha detected, renderer not configured")
			}
			// Fall through to Chrome renderer below.
		} else if results := parseYandexDataState(string(data)); len(results) > 0 {
			if len(results) > max {
				results = results[:max]
			}
			return results, nil
		}
	}

	// Fallback to Chrome render if available.
	if y.Renderer == nil {
		return nil, fmt.Errorf("yandex: no results from HTTP, renderer not configured")
	}
	renderedHTML, err := y.Renderer.Render(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("yandex render: %w", err)
	}
	results := parseYandexDataState(renderedHTML)
	if len(results) > max {
		results = results[:max]
	}
	return results, nil
}

type yandexState struct {
	InitialState struct {
		SerpList struct {
			Items struct {
				Entities map[string]yandexEntity `json:"entities"`
			} `json:"items"`
		} `json:"serpList"`
	} `json:"initialState"`
}

type yandexEntity struct {
	OrigURL    string        `json:"origUrl"`
	Image      string        `json:"image"`
	OrigWidth  int           `json:"origWidth"`
	OrigHeight int           `json:"origHeight"`
	Snippet    yandexSnippet `json:"snippet"`
	URL        string        `json:"url"` // relative URL with img_url param
}

type yandexSnippet struct {
	Title  string `json:"title"`
	Domain string `json:"domain"`
}

type yandexViewerState struct {
	ViewerData struct {
		Dups []yandexDup `json:"dups"`
	} `json:"viewerData"`
}

type yandexDup struct {
	URL        string      `json:"url"`
	W          int         `json:"w"`
	H          int         `json:"h"`
	Title      string      `json:"title"`
	SourceName string      `json:"sourceName"`
	SourceURL  string      `json:"sourceUrl"`
	Thumb      yandexThumb `json:"thumb"`
}

type yandexThumb struct {
	URL string `json:"url"`
}

// parseYandexDataState finds the data-state block containing initialState.serpList
// and extracts image results from entities.
func parseYandexDataState(pageHTML string) []ImageResult {
	matches := dataStateRe.FindAllStringSubmatch(pageHTML, -1)

	for _, m := range matches {
		decoded := html.UnescapeString(m[1])

		var state yandexState
		if err := json.Unmarshal([]byte(decoded), &state); err != nil {
			continue
		}

		entities := state.InitialState.SerpList.Items.Entities
		if len(entities) == 0 {
			// Try viewerData.dups path (alternative Yandex response format).
			var viewerState yandexViewerState
			if err := json.Unmarshal([]byte(decoded), &viewerState); err == nil {
				if dups := viewerState.ViewerData.Dups; len(dups) > 0 {
					var results []ImageResult
					for _, d := range dups {
						if d.URL == "" {
							continue
						}
						thumb := d.Thumb.URL
						if thumb != "" && strings.HasPrefix(thumb, "//") {
							thumb = "https:" + thumb
						}
						results = append(results, ImageResult{
							URL:       d.URL,
							Thumbnail: thumb,
							Source:    d.SourceURL,
							Title:     d.Title,
							Width:     d.W,
							Height:    d.H,
							Engine:    "yandex",
						})
					}
					if len(results) > 0 {
						return results
					}
				}
			}
			continue
		}

		var results []ImageResult
		for _, e := range entities {
			if e.OrigURL == "" {
				continue
			}

			thumb := e.Image
			if thumb != "" && strings.HasPrefix(thumb, "//") {
				thumb = "https:" + thumb
			}

			source := extractImgURL(e.URL)
			if source == "" {
				source = "https://" + e.Snippet.Domain
			}

			results = append(results, ImageResult{
				URL:       e.OrigURL,
				Thumbnail: thumb,
				Source:    source,
				Title:     e.Snippet.Title,
				Width:     e.OrigWidth,
				Height:    e.OrigHeight,
				Engine:    "yandex",
			})
		}
		return results
	}

	return nil
}

// extractImgURL extracts the img_url parameter from a Yandex relative URL.
func extractImgURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	// rawURL looks like: /images/search?pos=3&img_url=https%3A%2F%2Fexample.com%2Fphoto.jpg&...
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("img_url")
}
