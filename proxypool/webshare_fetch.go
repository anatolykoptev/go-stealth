package proxypool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// errorBodyLimit bounds the API error body copied into the returned error —
// the refresher hits this endpoint periodically, so an unbounded body cannot
// be allowed to grow the error (and its log lines) without limit.
const errorBodyLimit = 64 << 10

// fetchAllProxies retrieves all pages from the Webshare API.
// It stops after webshareMaxPages pages to guard against infinite loops.
func fetchAllProxies(ctx context.Context, apiKey string, cfg WebshareConfig) ([]webshareProxy, error) {
	firstURL := buildAPIURL(cfg)
	client := cfg.HTTPClient

	var all []webshareProxy
	nextURL := firstURL
	for range webshareMaxPages {
		page, next, err := fetchPageWithNext(ctx, client, nextURL, apiKey)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		if err := checkSameOrigin(firstURL, next); err != nil {
			return nil, err
		}
		nextURL = next
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("proxy: webshare returned 0 proxies")
	}
	return all, nil
}

// noRedirectClient returns a shallow copy of c that never follows redirects,
// so the Authorization header cannot be replayed to a redirect target.
// The caller's client is left untouched.
func noRedirectClient(c *http.Client) *http.Client {
	if c == nil {
		c = &http.Client{}
	}
	cp := *c
	cp.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &cp
}

// checkSameOrigin rejects a pagination link whose scheme or host:port differs
// from the first page, because every page request carries the API key.
func checkSameOrigin(first, next string) error {
	fu, err := url.Parse(first)
	if err != nil {
		return fmt.Errorf("proxy: parse first page URL: %w", err)
	}
	nu, err := url.Parse(next)
	if err != nil {
		return fmt.Errorf("proxy: parse next page URL: %w", err)
	}
	if !strings.EqualFold(fu.Scheme, nu.Scheme) ||
		!strings.EqualFold(fu.Hostname(), nu.Hostname()) ||
		effectivePort(fu) != effectivePort(nu) {
		return fmt.Errorf("proxy: refusing cross-origin next page link (%s://%s)", nu.Scheme, nu.Host)
	}
	return nil
}

// effectivePort returns the explicit port, or the scheme default.
func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

// fetchPage fetches a single API page and returns raw proxy entries (no next link).
func fetchPage(ctx context.Context, client *http.Client, apiURL, apiKey string) ([]webshareProxy, error) {
	proxies, _, err := fetchPageWithNext(ctx, client, apiURL, apiKey)
	return proxies, err
}

// fetchPageWithNext fetches one page and returns proxies + the next-page URL (empty if none).
func fetchPageWithNext(ctx context.Context, client *http.Client, apiURL, apiKey string) ([]webshareProxy, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("proxy: build request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+apiKey)

	resp, err := noRedirectClient(client).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("proxy: fetch list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return nil, "", fmt.Errorf("proxy: webshare API returned %d: %s", resp.StatusCode, string(body))
	}

	var data webshareResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, "", fmt.Errorf("proxy: decode response: %w", err)
	}

	next := ""
	if data.Next != nil {
		next = *data.Next
	}
	return data.Results, next, nil
}
