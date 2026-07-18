package session

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"sync"
	"sync/atomic"
	"time"

	stealth "github.com/anatolykoptev/go-stealth"
)

// Session tracks stateful browsing context: cookie jar, request count,
// timestamps, and a fixed browser profile. Each session maintains its own
// BrowserClient with a consistent fingerprint.
type Session struct {
	ID           string
	CreatedAt    time.Time
	lastUsed     time.Time
	requestCount atomic.Int64
	client       *stealth.BrowserClient
	profile      stealth.BrowserProfile
	mu           sync.RWMutex
}

// Option configures a Session.
type Option func(*sessionConfig)

type sessionConfig struct {
	profile    *stealth.BrowserProfile
	clientOpts []stealth.ClientOption
}

// WithProfile sets a fixed browser profile for the session.
// If not specified, a random platform-matched profile is used.
func WithProfile(p stealth.BrowserProfile) Option {
	return func(c *sessionConfig) {
		c.profile = &p
	}
}

// WithClientOptions passes additional options to the underlying BrowserClient.
func WithClientOptions(opts ...stealth.ClientOption) Option {
	return func(c *sessionConfig) {
		c.clientOpts = append(c.clientOpts, opts...)
	}
}

// New creates a Session with a consistent browser fingerprint.
// The session maintains cookies, tracks request count, and uses a fixed profile.
func New(opts ...Option) (*Session, error) {
	cfg := &sessionConfig{}
	for _, o := range opts {
		o(cfg)
	}

	var profile stealth.BrowserProfile
	if cfg.profile != nil {
		profile = *cfg.profile
	} else {
		profile = stealth.PlatformMatchedProfile()
	}

	clientOpts := append([]stealth.ClientOption{
		stealth.WithProfile(profile.TLSProfile),
		stealth.WithFollowRedirects(),
	}, cfg.clientOpts...)

	client, err := stealth.NewClient(clientOpts...)
	if err != nil {
		return nil, err
	}

	return &Session{
		ID:        generateID(),
		CreatedAt: time.Now(),
		lastUsed:  time.Now(),
		client:    client,
		profile:   profile,
	}, nil
}

// Do executes an HTTP request within this session.
// The profile's User-Agent is set automatically if not provided in headers.
// Request count and last-used time are updated atomically.
func (s *Session) Do(method, url string, headers map[string]string, body io.Reader) ([]byte, map[string]string, int, error) {
	if headers == nil {
		headers = make(map[string]string)
	}
	if _, ok := headers["user-agent"]; !ok {
		headers["user-agent"] = s.profile.UserAgent
	}

	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()

	s.requestCount.Add(1)

	return s.client.Do(method, url, headers, body)
}

// Client returns the underlying BrowserClient for direct access.
func (s *Session) Client() *stealth.BrowserClient {
	return s.client
}

// Profile returns the session's browser profile.
func (s *Session) Profile() stealth.BrowserProfile {
	return s.profile
}

// RequestCount returns the total number of requests made in this session.
func (s *Session) RequestCount() int64 {
	return s.requestCount.Load()
}

// LastUsed returns the time of the last request.
func (s *Session) LastUsed() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastUsed
}

// Age returns how long this session has existed.
func (s *Session) Age() time.Duration {
	return time.Since(s.CreatedAt)
}

// IdleTime returns how long since the last request.
func (s *Session) IdleTime() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.lastUsed)
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
