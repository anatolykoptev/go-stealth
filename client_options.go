package stealth

import (
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// ClientOption configures a BrowserClient.
type ClientOption func(*clientConfig)

type clientConfig struct {
	proxyURL     string
	profile      profiles.ClientProfile
	timeout      int
	headerOrder  []string
	cookieJar    tls_client.CookieJar
	followRedirs bool
}

func defaultConfig() *clientConfig {
	return &clientConfig{
		profile: profiles.Chrome_131,
		timeout: 20,
	}
}

// WithProxy sets the proxy URL (e.g. "socks5://user:pass@host:port").
func WithProxy(url string) ClientOption {
	return func(c *clientConfig) {
		c.proxyURL = url
	}
}

// WithProfile sets the TLS client profile.
func WithProfile(p profiles.ClientProfile) ClientOption {
	return func(c *clientConfig) {
		c.profile = p
	}
}

// WithTimeout sets the request timeout in seconds.
func WithTimeout(seconds int) ClientOption {
	return func(c *clientConfig) {
		c.timeout = seconds
	}
}

// WithHeaderOrder sets the default HTTP header ordering for requests.
func WithHeaderOrder(order []string) ClientOption {
	return func(c *clientConfig) {
		c.headerOrder = order
	}
}

// WithCookieJar sets a custom cookie jar.
func WithCookieJar(jar tls_client.CookieJar) ClientOption {
	return func(c *clientConfig) {
		c.cookieJar = jar
	}
}

// WithFollowRedirects enables redirect following (disabled by default).
func WithFollowRedirects() ClientOption {
	return func(c *clientConfig) {
		c.followRedirs = true
	}
}
