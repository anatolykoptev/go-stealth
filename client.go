package stealth

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"

	"github.com/anatolykoptev/go-stealth/proxypool"
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
	proxyPool   proxypool.ProxyPool // nil = no auto-rotation
	middlewares []Middleware
	handler     Handler // lazy-built from middlewares + base handler
	debug       bool
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

	bc := &BrowserClient{
		client:      client,
		headerOrder: order,
		proxyPool:   cfg.proxyPool,
		debug:       cfg.debug,
	}
	if cfg.debug {
		bc.Use(LoggingMiddleware)
	}
	return bc, nil
}

// Use appends middlewares to the client's middleware chain.
// Middlewares execute in the order they are added (first added = outermost).
func (bc *BrowserClient) Use(mw ...Middleware) {
	bc.middlewares = append(bc.middlewares, mw...)
	bc.handler = nil // rebuild on next Do()
}

// buildHandler constructs the handler chain from middlewares + base handler.
func (bc *BrowserClient) buildHandler() Handler {
	if bc.handler != nil {
		return bc.handler
	}
	base := bc.baseHandler(bc.headerOrder)
	if len(bc.middlewares) > 0 {
		bc.handler = Chain(bc.middlewares...)(base)
	} else {
		bc.handler = base
	}
	return bc.handler
}

// baseHandler returns the core Handler that executes TLS-fingerprinted HTTP requests.
func (bc *BrowserClient) baseHandler(order []string) Handler {
	return func(req *Request) (*Response, error) {
		httpReq, err := fhttp.NewRequest(req.Method, req.URL, req.Body)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}

		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}
		httpReq.Header[fhttp.HeaderOrderKey] = order

		resp, err := bc.client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("tls request: %w", err)
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return &Response{StatusCode: resp.StatusCode}, fmt.Errorf("read body: %w", err)
		}

		respHeaders := make(map[string]string, len(resp.Header))
		for k, v := range resp.Header {
			if strings.ToLower(k) == "set-cookie" {
				respHeaders["set-cookie"] = strings.Join(v, "; ")
			} else if len(v) > 0 {
				respHeaders[strings.ToLower(k)] = v[0]
			}
		}

		return &Response{Body: data, Headers: respHeaders, StatusCode: resp.StatusCode}, nil
	}
}

// Do executes an HTTP request with TLS fingerprint impersonation.
// Returns (body bytes, response headers, HTTP status code, error).
// Response headers are returned with lowercase canonical keys.
// If a ProxyPool was configured via WithProxyPool, each call rotates to the next proxy.
// Middleware added via Use() is applied to each request.
func (bc *BrowserClient) Do(method, urlStr string, headers map[string]string, body io.Reader) ([]byte, map[string]string, int, error) {
	if bc.proxyPool != nil {
		proxyURL := bc.proxyPool.Next()
		if err := bc.SetProxy(proxyURL); err != nil {
			slog.Warn("proxy: SetProxy failed", slog.String("proxy", MaskProxy(proxyURL)), slog.Any("error", err))
		}
	}

	req := &Request{Method: method, URL: urlStr, Headers: headers, Body: body}
	handler := bc.buildHandler()

	resp, err := handler(req)
	if err != nil {
		if resp != nil {
			return nil, nil, resp.StatusCode, err
		}
		return nil, nil, 0, err
	}

	return resp.Body, resp.Headers, resp.StatusCode, nil
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
// Middleware is applied. Proxy rotation is applied.
func (bc *BrowserClient) DoWithHeaderOrder(method, urlStr string, headers map[string]string, body io.Reader, order []string) ([]byte, map[string]string, int, error) {
	if bc.proxyPool != nil {
		proxyURL := bc.proxyPool.Next()
		if err := bc.SetProxy(proxyURL); err != nil {
			slog.Warn("proxy: SetProxy failed", slog.String("proxy", MaskProxy(proxyURL)), slog.Any("error", err))
		}
	}

	req := &Request{Method: method, URL: urlStr, Headers: headers, Body: body}

	// Use base handler with custom order, wrapped in middleware
	base := bc.baseHandler(order)
	var handler Handler
	if len(bc.middlewares) > 0 {
		handler = Chain(bc.middlewares...)(base)
	} else {
		handler = base
	}

	resp, err := handler(req)
	if err != nil {
		if resp != nil {
			return nil, nil, resp.StatusCode, err
		}
		return nil, nil, 0, err
	}

	return resp.Body, resp.Headers, resp.StatusCode, nil
}
