package stealth

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryTracker_ShouldRetry_NewURL(t *testing.T) {
	rt := NewRetryTracker(3, time.Minute)
	if !rt.ShouldRetry("http://example.com") {
		t.Error("new URL should be retryable")
	}
}

func TestRetryTracker_ExceedsMaxAttempts(t *testing.T) {
	rt := NewRetryTracker(2, time.Minute)
	rt.RecordAttempt("http://a.com", &HttpStatusError{StatusCode: 503})
	rt.RecordAttempt("http://a.com", &HttpStatusError{StatusCode: 503})
	if rt.ShouldRetry("http://a.com") {
		t.Error("should not retry after max attempts exceeded")
	}
}

func TestRetryTracker_PermanentError(t *testing.T) {
	rt := NewRetryTracker(5, time.Minute)
	rt.RecordAttempt("http://a.com", &HttpStatusError{StatusCode: http.StatusNotFound})
	if rt.ShouldRetry("http://a.com") {
		t.Error("404 should mark URL as permanent failure")
	}
}

func TestRetryTracker_RecordSuccess_Clears(t *testing.T) {
	rt := NewRetryTracker(3, time.Minute)
	rt.RecordAttempt("http://a.com", &HttpStatusError{StatusCode: 503})
	rt.RecordSuccess("http://a.com")
	if !rt.ShouldRetry("http://a.com") {
		t.Error("successful URL should be retryable again")
	}
}

func TestRetryTracker_TTLEviction(t *testing.T) {
	rt := NewRetryTracker(1, 10*time.Millisecond)
	rt.RecordAttempt("http://old.com", &HttpStatusError{StatusCode: 503})
	if rt.ShouldRetry("http://old.com") {
		t.Error("should not retry immediately after max attempts")
	}
	time.Sleep(20 * time.Millisecond)
	if !rt.ShouldRetry("http://old.com") {
		t.Error("should retry after TTL eviction")
	}
}

func TestRetryTracker_PermanentStatusCodes(t *testing.T) {
	permanentCodes := []int{400, 401, 403, 404, 410, 451}
	for _, code := range permanentCodes {
		rt := NewRetryTracker(5, time.Minute)
		rt.RecordAttempt("http://test.com", &HttpStatusError{StatusCode: code})
		if rt.ShouldRetry("http://test.com") {
			t.Errorf("HTTP %d should be permanent failure", code)
		}
	}
}

func TestRetryTracker_ConcurrentAccess(t *testing.T) {
	rt := NewRetryTracker(100, time.Minute)
	done := make(chan struct{})
	for i := range 10 {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			url := "http://example.com/" + string(rune('a'+id))
			for range 50 {
				rt.ShouldRetry(url)
				rt.RecordAttempt(url, &HttpStatusError{StatusCode: 503})
				rt.RecordSuccess(url)
			}
		}(i)
	}
	for range 10 {
		<-done
	}
}
