package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestDomainLimiter_Allow(t *testing.T) {
	dl := NewDomainLimiter(DomainConfig{
		Domain:            "api.example.com",
		RequestsPerWindow: 3,
		WindowDuration:    1 * time.Minute,
	})

	for i := 0; i < 3; i++ {
		if !dl.Allow("https://api.example.com/v1/users") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	if dl.Allow("https://api.example.com/v1/users") {
		t.Fatal("4th request should be denied")
	}
}

func TestDomainLimiter_MinDelay(t *testing.T) {
	dl := NewDomainLimiter(DomainConfig{
		Domain:            "slow.example.com",
		RequestsPerWindow: 100,
		WindowDuration:    1 * time.Minute,
		MinDelay:          50 * time.Millisecond,
	})

	if !dl.Allow("https://slow.example.com/page") {
		t.Fatal("first request should be allowed")
	}

	// Immediate second request should be denied
	if dl.Allow("https://slow.example.com/page") {
		t.Fatal("request too soon should be denied")
	}

	time.Sleep(60 * time.Millisecond)

	if !dl.Allow("https://slow.example.com/page") {
		t.Fatal("request after delay should be allowed")
	}
}

func TestDomainLimiter_WildcardMatch(t *testing.T) {
	dl := NewDomainLimiter(DomainConfig{
		Domain:            "*.twitter.com",
		RequestsPerWindow: 2,
		WindowDuration:    1 * time.Minute,
	})

	if !dl.Allow("https://api.twitter.com/graphql") {
		t.Fatal("first request to api.twitter.com should be allowed")
	}
	if !dl.Allow("https://api.twitter.com/graphql") {
		t.Fatal("second request to api.twitter.com should be allowed")
	}
	if dl.Allow("https://api.twitter.com/graphql") {
		t.Fatal("third request to api.twitter.com should be denied")
	}

	// Different subdomain should have its own counter
	if !dl.Allow("https://upload.twitter.com/media") {
		t.Fatal("first request to upload.twitter.com should be allowed")
	}
}

func TestDomainLimiter_NoRuleAllows(t *testing.T) {
	dl := NewDomainLimiter(DomainConfig{
		Domain:            "restricted.com",
		RequestsPerWindow: 1,
		WindowDuration:    1 * time.Minute,
	})

	// Unmatched domain should always be allowed
	for i := 0; i < 10; i++ {
		if !dl.Allow("https://unrestricted.com/page") {
			t.Fatalf("request %d to unmatched domain should be allowed", i+1)
		}
	}
}

func TestDomainLimiter_Wait(t *testing.T) {
	dl := NewDomainLimiter(DomainConfig{
		Domain:            "wait.example.com",
		RequestsPerWindow: 100,
		WindowDuration:    1 * time.Minute,
		MinDelay:          30 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := dl.Wait(ctx, "https://wait.example.com/page"); err != nil {
		t.Fatalf("first wait should succeed: %v", err)
	}

	// Second wait should block briefly then succeed
	if err := dl.Wait(ctx, "https://wait.example.com/page"); err != nil {
		t.Fatalf("second wait should succeed: %v", err)
	}
}

func TestDomainLimiter_WaitCancelled(t *testing.T) {
	dl := NewDomainLimiter(DomainConfig{
		Domain:            "block.example.com",
		RequestsPerWindow: 1,
		WindowDuration:    1 * time.Hour,
	})

	dl.Allow("https://block.example.com/page")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := dl.Wait(ctx, "https://block.example.com/page")
	if err == nil {
		t.Fatal("expected context timeout error")
	}
}

func TestDomainLimiter_CatchallRule(t *testing.T) {
	dl := NewDomainLimiter(DomainConfig{
		Domain:            "",
		RequestsPerWindow: 2,
		WindowDuration:    1 * time.Minute,
	})

	if !dl.Allow("https://anything.com/page") {
		t.Fatal("first request should be allowed")
	}
	if !dl.Allow("https://anything.com/page") {
		t.Fatal("second request should be allowed")
	}
	if dl.Allow("https://anything.com/page") {
		t.Fatal("third request should be denied by catchall")
	}
}

func TestDomainLimiter_RandomDelay(t *testing.T) {
	dl := NewDomainLimiter(DomainConfig{
		Domain:            "rand.example.com",
		RequestsPerWindow: 100,
		WindowDuration:    1 * time.Minute,
		MinDelay:          10 * time.Millisecond,
		RandomDelay:       50 * time.Millisecond,
	})

	if !dl.Allow("https://rand.example.com/page") {
		t.Fatal("first request should be allowed")
	}

	// Immediate second request should be denied (MinDelay + RandomDelay not elapsed)
	if dl.Allow("https://rand.example.com/page") {
		t.Fatal("request too soon should be denied")
	}

	// After max possible delay (MinDelay + RandomDelay = 60ms), should be allowed
	time.Sleep(70 * time.Millisecond)

	if !dl.Allow("https://rand.example.com/page") {
		t.Fatal("request after full delay should be allowed")
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://api.twitter.com/graphql?q=test", "api.twitter.com"},
		{"http://EXAMPLE.COM/page", "example.com"},
		{"https://host:8080/path", "host"},
		{"not-a-url", "not-a-url"},
	}
	for _, tt := range tests {
		got := extractDomain(tt.input)
		if got != tt.want {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
