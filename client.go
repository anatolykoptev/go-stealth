package stealth

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
)

// DefaultHeaderOrder is a generic Chrome-like header order.
var DefaultHeaderOrder = []string{
	"accept",
	"accept-language",
	"accept-encoding",
	"referer",
	"cookie",
	"user-agent",
}

// BrowserClient wraps bogdanfinn/tls-client with TLS fingerprint impersonation.
type BrowserClient struct {
	client      tls_client.HttpClient
	headerOrder []string
}

// NewClient creates a BrowserClient with the given options.
func NewClient(opts ...ClientOption) (*BrowserClient, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	jar := cfg.cookieJar
	if jar == nil {
		jar = tls_client.NewCookieJar()
	}

	tlsOpts := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(cfg.timeout),
		tls_client.WithClientProfile(cfg.profile),
		tls_client.WithCookieJar(jar),
		tls_client.WithInsecureSkipVerify(),
	}
	if !cfg.followRedirs {
		tlsOpts = append(tlsOpts, tls_client.WithNotFollowRedirects())
	}
	if cfg.proxyURL != "" {
		tlsOpts = append(tlsOpts, tls_client.WithProxyUrl(cfg.proxyURL))
	}

	client, err := tls_client.NewHttpClient(nil, tlsOpts...)
	if err != nil {
		return nil, fmt.Errorf("tls-client init: %w", err)
	}

	order := cfg.headerOrder
	if order == nil {
		order = DefaultHeaderOrder
	}

	return &BrowserClient{client: client, headerOrder: order}, nil
}

// Do executes an HTTP request with TLS fingerprint impersonation.
// Returns (body bytes, response headers, HTTP status code, error).
// Response headers are returned with lowercase canonical keys.
func (bc *BrowserClient) Do(method, urlStr string, headers map[string]string, body io.Reader) ([]byte, map[string]string, int, error) {
	req, err := fhttp.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("build request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	req.Header[fhttp.HeaderOrderKey] = bc.headerOrder

	resp, err := bc.client.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("tls request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	respHeaders := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		if strings.ToLower(k) == "set-cookie" {
			respHeaders["set-cookie"] = strings.Join(v, "; ")
		} else if len(v) > 0 {
			respHeaders[strings.ToLower(k)] = v[0]
		}
	}

	return data, respHeaders, resp.StatusCode, nil
}

// SetProxy changes the proxy URL for subsequent requests.
func (bc *BrowserClient) SetProxy(proxyURL string) error {
	return bc.client.SetProxy(proxyURL)
}

// GetCookieValue returns the value of a named cookie for the given URL from the jar.
func (bc *BrowserClient) GetCookieValue(rawURL, name string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, c := range bc.client.GetCookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// DoWithHeaderOrder executes a request with a custom header order, overriding the default.
func (bc *BrowserClient) DoWithHeaderOrder(method, urlStr string, headers map[string]string, body io.Reader, order []string) ([]byte, map[string]string, int, error) {
	req, err := fhttp.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("build request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	req.Header[fhttp.HeaderOrderKey] = order

	resp, err := bc.client.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("tls request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	respHeaders := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		if strings.ToLower(k) == "set-cookie" {
			respHeaders["set-cookie"] = strings.Join(v, "; ")
		} else if len(v) > 0 {
			respHeaders[strings.ToLower(k)] = v[0]
		}
	}

	return data, respHeaders, resp.StatusCode, nil
}
