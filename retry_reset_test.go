package stealth

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryDoWithReset_CallsResetBeforeRetry(t *testing.T) {
	var resets, attempts atomic.Int32
	rc := RetryConfig{MaxRetries: 3, InitialWait: time.Millisecond, MaxWait: 10 * time.Millisecond, Multiplier: 1}

	result, err := RetryDoWithReset(context.Background(), rc,
		func() { resets.Add(1) },
		func() (string, error) {
			n := attempts.Add(1)
			if n <= 2 {
				return "", &HttpStatusError{StatusCode: 503}
			}
			return "ok", nil
		})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q, want %q", result, "ok")
	}
	if got := resets.Load(); got != 2 {
		t.Errorf("resetFn called %d times, want 2 (once per retry, not before first attempt)", got)
	}
}

func TestRetryDoWithReset_NilResetFn(t *testing.T) {
	rc := RetryConfig{MaxRetries: 1, InitialWait: time.Millisecond, MaxWait: 10 * time.Millisecond, Multiplier: 1}

	// nil resetFn should not panic.
	_, err := RetryDoWithReset[int](context.Background(), rc, nil,
		func() (int, error) { return 0, &HttpStatusError{StatusCode: 500} })
	if err == nil {
		t.Error("expected error")
	}
}

func TestRetryHTTPWithReset_CallsReset(t *testing.T) {
	var resets atomic.Int32
	rc := RetryConfig{MaxRetries: 2, InitialWait: time.Millisecond, MaxWait: 10 * time.Millisecond, Multiplier: 1}

	_, _ = RetryHTTPWithReset(context.Background(), rc,
		func() { resets.Add(1) },
		func() (*http.Response, error) {
			return nil, &HttpStatusError{StatusCode: 502}
		})

	if got := resets.Load(); got != 2 {
		t.Errorf("resetFn called %d times, want 2", got)
	}
}
