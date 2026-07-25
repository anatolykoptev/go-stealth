package stealth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStdBackend_Do(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Header.Get("User-Agent")))
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithStdHTTP())
	if err != nil {
		t.Fatal(err)
	}

	body, _, status, err := client.Do("GET", server.URL, map[string]string{"user-agent": "test-agent"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if string(body) != "test-agent" {
		t.Fatalf("expected test-agent, got %q", string(body))
	}
}

func TestStdBackend_Post(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		w.Write(buf[:n])
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithStdHTTP())
	if err != nil {
		t.Fatal(err)
	}

	body, _, status, err := client.Do("POST", server.URL, nil, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if string(body) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(body))
	}
}

func TestStdBackend_Cookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("session"); err == nil {
			w.Write([]byte(c.Value))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithStdHTTP())
	if err != nil {
		t.Fatal(err)
	}

	// First request sets the cookie
	client.Do("GET", server.URL, nil, nil)

	// Second request sends it back
	body, _, _, _ := client.Do("GET", server.URL, nil, nil)
	if string(body) != "abc123" {
		t.Fatalf("expected cookie value abc123, got %q", string(body))
	}

	// GetCookieValue should find it
	v := client.GetCookieValue(server.URL, "session")
	if v != "abc123" {
		t.Fatalf("expected GetCookieValue=abc123, got %q", v)
	}
}

func TestStdBackend_NoRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/target", 302)
			return
		}
		w.Write([]byte("reached"))
	}))
	defer server.Close()

	// Default: no redirects
	client, err := NewClient(WithoutSSRFGuard(), WithStdHTTP())
	if err != nil {
		t.Fatal(err)
	}

	_, _, status, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 302 {
		t.Fatalf("expected 302, got %d", status)
	}
}

func TestStdBackend_FollowRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/target", 302)
			return
		}
		w.Write([]byte("reached"))
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithStdHTTP(), WithFollowRedirects())
	if err != nil {
		t.Fatal(err)
	}

	body, _, status, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if string(body) != "reached" {
		t.Fatalf("expected 'reached', got %q", string(body))
	}
}

func TestStdBackend_WithMiddleware(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Injected") != "yes" {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithStdHTTP())
	if err != nil {
		t.Fatal(err)
	}

	client.Use(func(next Handler) Handler {
		return func(req *Request) (*Response, error) {
			if req.Headers == nil {
				req.Headers = make(map[string]string)
			}
			req.Headers["X-Injected"] = "yes"
			return next(req)
		}
	})

	_, _, status, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
}

func TestCustomBackend(t *testing.T) {
	// Test WithBackend with a custom factory
	customFactory := func(cfg BackendConfig) (HTTPDoer, error) {
		return newStdBackend(cfg)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("custom"))
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithBackend(customFactory))
	if err != nil {
		t.Fatal(err)
	}

	body, _, _, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "custom" {
		t.Fatalf("expected 'custom', got %q", string(body))
	}
}

func TestTLSProfileConstants(t *testing.T) {
	// Verify all profile constants map to valid bogdanfinn profiles
	profiles := []TLSProfile{
		ProfileChrome131, ProfileChrome133,
		ProfileChrome144, ProfileChrome146,
		ProfileFirefox133,
		ProfileSafari16, ProfileSafariIOS18, ProfileSafariIOS17,
	}
	for _, p := range profiles {
		mapped := mapTLSProfile(p)
		if mapped.GetClientHelloStr() == "" {
			t.Errorf("profile %s mapped to empty client hello", p)
		}
	}
}

// TestTLSProfile_NewChromeMapsToCorrectBogdanfinnProfile verifies the new
// Chrome 144/146 constants resolve to the matching bogdanfinn profile THROUGH
// the existing mapTLSProfile (not a re-implemented map), by comparing the
// resolved ClientHello version string.
func TestTLSProfile_NewChromeMapsToCorrectBogdanfinnProfile(t *testing.T) {
	tests := []struct {
		goStealthProfile TLSProfile
		wantVersion      string
	}{
		{ProfileChrome144, "144"},
		{ProfileChrome146, "146"},
	}
	for _, tt := range tests {
		mapped := mapTLSProfile(tt.goStealthProfile)
		got := mapped.GetClientHelloStr()
		// bogdanfinn encodes the version in the ClientHello "Version" field,
		// surfaced via GetClientHelloStr as "Chrome/<version>".
		if !strings.Contains(got, "Chrome") || !strings.Contains(got, tt.wantVersion) {
			t.Errorf("mapTLSProfile(%q) = %q, want a Chrome profile with version %q",
				tt.goStealthProfile, got, tt.wantVersion)
		}
	}
}

func TestTLSProfileFallback(t *testing.T) {
	// Unknown profile should fall back to Chrome_131
	mapped := mapTLSProfile("unknown_profile")
	defaultMapped := mapTLSProfile(ProfileChrome131)
	if mapped.GetClientHelloStr() != defaultMapped.GetClientHelloStr() {
		t.Fatalf("expected fallback to Chrome_131")
	}
}

func TestBuiltinProfilesUseTLSProfile(t *testing.T) {
	// All builtin profiles should use our TLSProfile constants
	for i, p := range BuiltinProfiles {
		if p.TLSProfile == "" {
			t.Errorf("profile %d has empty TLSProfile", i)
		}
		if _, ok := profileMap[p.TLSProfile]; !ok {
			t.Errorf("profile %d uses unmapped TLSProfile %q", i, p.TLSProfile)
		}
	}
}
