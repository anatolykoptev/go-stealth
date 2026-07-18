package uri

import "testing"

func TestExtractHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"https://api.twitter.com/graphql?q=test", "api.twitter.com"},
		{"http://EXAMPLE.COM/page", "example.com"},
		{"https://host:8080/path", "host"},
		{"not-a-url", "not-a-url"},
		{"example.com", "example.com"},
		{"https://example.com:8080/path/to/page", "example.com"},
		{"://weird", "://weird"},
		{"", ""},
		{"https://host.example.org/path", "host.example.org"},
		{"http://api.example.com:9000", "api.example.com"},
		{"https://sub.example.com:8080/path", "sub.example.com"},
		{"https://example.com", "example.com"},
	}
	for _, tt := range tests {
		got := ExtractHost(tt.input)
		if got != tt.want {
			t.Errorf("ExtractHost(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
