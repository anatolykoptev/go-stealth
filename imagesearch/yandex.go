package imagesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
)

const yandexImagesURL = "https://yandex.ru/images/search"

var dataStateRe = regexp.MustCompile(`data-state="([^"]+)"`)

// YandexImages searches Yandex Images via Chrome render + data-state JSON parsing.
// Requires PageRenderer (go-browser). Silently returns nil when Renderer is nil.
type YandexImages struct {
	Renderer PageRenderer
}

func (y *YandexImages) Name() string { return "yandex" }

func (y *YandexImages) Search(ctx context.Context, _ BrowserDoer, query string, max int) ([]ImageResult, error) {
	if y.Renderer == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	u := yandexImagesURL + "?text=" + url.QueryEscape(query)
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
	OrigURL   string `json:"origUrl"`
	ThumbURL  string `json:"thumbUrl"`
	Title     string `json:"title"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SourceURL string `json:"sourceUrl"`
}

func parseYandexDataState(pageHTML string) []ImageResult {
	m := dataStateRe.FindStringSubmatch(pageHTML)
	if len(m) < 2 {
		return nil
	}

	decoded := html.UnescapeString(m[1])
	var state yandexState
	if err := json.Unmarshal([]byte(decoded), &state); err != nil {
		return nil
	}

	var results []ImageResult
	for _, e := range state.InitialState.SerpList.Items.Entities {
		if e.OrigURL == "" {
			continue
		}
		results = append(results, ImageResult{
			URL:       e.OrigURL,
			Thumbnail: e.ThumbURL,
			Source:    e.SourceURL,
			Title:     e.Title,
			Width:     e.Width,
			Height:    e.Height,
			Engine:    "yandex",
		})
	}
	return results
}
