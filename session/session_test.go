package session

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	stealth "github.com/anatolykoptev/go-stealth"
)

func TestNew_DefaultProfile(t *testing.T) {
	s, err := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	if err != nil {
		t.Fatal(err)
	}
	if s.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if s.Profile().UserAgent == "" {
		t.Fatal("expected non-empty profile UA")
	}
	if s.Profile().Browser == "" {
		t.Fatal("expected non-empty browser metadata")
	}
	if s.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt")
	}
}

func TestNew_WithProfile(t *testing.T) {
	p := stealth.RandomProfile(stealth.WithBrowser("firefox"), stealth.WithOS("linux"))
	s, err := New(WithProfile(p), WithClientOptions(stealth.WithoutSSRFGuard()))
	if err != nil {
		t.Fatal(err)
	}
	if s.Profile().Browser != "firefox" {
		t.Fatalf("expected firefox, got %s", s.Profile().Browser)
	}
}

func TestSession_Do(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua == "" {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(ua))
	}))
	defer server.Close()

	s, err := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	if err != nil {
		t.Fatal(err)
	}

	body, _, status, err := s.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	// Should auto-set User-Agent from profile
	if !strings.Contains(string(body), "Mozilla") {
		t.Fatalf("expected Mozilla UA, got: %s", body)
	}
}

func TestSession_RequestCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	s, err := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	if err != nil {
		t.Fatal(err)
	}

	if s.RequestCount() != 0 {
		t.Fatalf("expected 0 requests, got %d", s.RequestCount())
	}

	for range 5 {
		s.Do("GET", server.URL, nil, nil)
	}

	if s.RequestCount() != 5 {
		t.Fatalf("expected 5 requests, got %d", s.RequestCount())
	}
}

func TestSession_LastUsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	s, err := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now()
	time.Sleep(5 * time.Millisecond)
	s.Do("GET", server.URL, nil, nil)

	if s.LastUsed().Before(before) {
		t.Fatal("LastUsed should be updated after Do()")
	}
}

func TestSession_Age(t *testing.T) {
	s, err := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond)
	if s.Age() < 5*time.Millisecond {
		t.Fatalf("expected age >= 5ms, got %v", s.Age())
	}
}

func TestSession_IdleTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	s, err := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	if err != nil {
		t.Fatal(err)
	}

	s.Do("GET", server.URL, nil, nil)
	time.Sleep(10 * time.Millisecond)
	if s.IdleTime() < 10*time.Millisecond {
		t.Fatalf("expected idle >= 10ms, got %v", s.IdleTime())
	}
}

func TestSession_ConsistentProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.Header.Get("User-Agent")))
	}))
	defer server.Close()

	s, err := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	if err != nil {
		t.Fatal(err)
	}

	var firstUA string
	for range 5 {
		body, _, _, _ := s.Do("GET", server.URL, nil, nil)
		if firstUA == "" {
			firstUA = string(body)
		} else if string(body) != firstUA {
			t.Fatalf("UA changed across requests: %s vs %s", firstUA, string(body))
		}
	}
}

func TestSession_CustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	s, err := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	if err != nil {
		t.Fatal(err)
	}

	_, _, status, err := s.Do("GET", server.URL, map[string]string{"X-Custom": "value"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
}

func TestSession_UniqueIDs(t *testing.T) {
	s1, _ := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	s2, _ := New(WithClientOptions(stealth.WithoutSSRFGuard()))
	if s1.ID == s2.ID {
		t.Fatal("sessions should have unique IDs")
	}
}
