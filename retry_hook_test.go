package stealth

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryHook_CalledOnEachAttempt(t *testing.T) {
	var calls atomic.Int32
	hook := func(_ context.Context, attempt, maxAttempts int, err error) {
		calls.Add(1)
		if attempt < 1 || attempt > maxAttempts {
			t.Errorf("attempt=%d out of range [1, %d]", attempt, maxAttempts)
		}
		if err == nil {
			t.Error("err should not be nil on retry")
		}
	}

	rc := RetryConfig{
		MaxRetries:  3,
		InitialWait: time.Millisecond,
		MaxWait:     10 * time.Millisecond,
		Multiplier:  1,
	}
	ctx := WithRetryHook(context.Background(), hook)

	attempt := 0
	_, _ = RetryDo(ctx, rc, func() (int, error) {
		attempt++
		if attempt <= 3 {
			return 0, &HttpStatusError{StatusCode: http.StatusServiceUnavailable}
		}
		return 42, nil
	})

	if got := calls.Load(); got != 3 {
		t.Errorf("hook called %d times, want 3", got)
	}
}

func TestRetryHook_NilHookNoOp(t *testing.T) {
	rc := RetryConfig{
		MaxRetries:  1,
		InitialWait: time.Millisecond,
		MaxWait:     10 * time.Millisecond,
		Multiplier:  1,
	}
	// No hook in context — should not panic.
	_, err := RetryDo(context.Background(), rc, func() (int, error) {
		return 0, &HttpStatusError{StatusCode: http.StatusBadGateway}
	})
	if err == nil {
		t.Error("expected error")
	}
}

func TestRetryHook_WithRetryHTTP(t *testing.T) {
	var hookCalls atomic.Int32
	hook := func(_ context.Context, _, _ int, _ error) {
		hookCalls.Add(1)
	}

	rc := RetryConfig{
		MaxRetries:  2,
		InitialWait: time.Millisecond,
		MaxWait:     10 * time.Millisecond,
		Multiplier:  1,
	}
	ctx := WithRetryHook(context.Background(), hook)

	_, _ = RetryHTTP(ctx, rc, func() (*http.Response, error) {
		return nil, &HttpStatusError{StatusCode: http.StatusBadGateway}
	})

	if got := hookCalls.Load(); got != 2 {
		t.Errorf("hook called %d times, want 2", got)
	}
}
