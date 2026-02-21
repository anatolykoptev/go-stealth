package stealth

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfter_Seconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "120")

	got := ParseRetryAfter(resp)
	if got != 120*time.Second {
		t.Fatalf("expected 120s, got %v", got)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().Add(60 * time.Second).UTC().Format(time.RFC1123)
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", future)

	got := ParseRetryAfter(resp)
	// Allow 2s tolerance for clock drift
	if got < 58*time.Second || got > 62*time.Second {
		t.Fatalf("expected ~60s, got %v", got)
	}
}

func TestParseRetryAfter_Missing(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}

	got := ParseRetryAfter(resp)
	if got != 0 {
		t.Fatalf("expected 0 for missing header, got %v", got)
	}
}

func TestParseRetryAfter_PastDate(t *testing.T) {
	past := time.Now().Add(-60 * time.Second).UTC().Format(time.RFC1123)
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", past)

	got := ParseRetryAfter(resp)
	if got != 0 {
		t.Fatalf("expected 0 for past date, got %v", got)
	}
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "not-a-number")

	got := ParseRetryAfter(resp)
	if got != 0 {
		t.Fatalf("expected 0 for invalid header, got %v", got)
	}
}
