package stealth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTLSClient_VerifiesByDefault confirms the default tls-client backend
// rejects a self-signed httptest TLS certificate (secure-by-default). No
// WithInsecureSkipVerify is passed, so cert verification must fail.
func TestTLSClient_VerifiesByDefault(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard())
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = client.Do("GET", server.URL, nil, nil)
	if err == nil {
		t.Fatal("expected TLS verification error for self-signed cert, got nil")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "certificate") && !strings.Contains(low, "x509") && !strings.Contains(low, "authority") {
		t.Fatalf("expected error mentioning certificate/x509/authority, got: %v", err)
	}
}

// TestTLSClient_SkipVerifyOptIn confirms WithInsecureSkipVerify opts in to
// bypassing self-signed cert verification on the tls-client backend.
func TestTLSClient_SkipVerifyOptIn(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithInsecureSkipVerify())
	if err != nil {
		t.Fatal(err)
	}

	_, _, status, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatalf("expected success with skip-verify, got error: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}
}

// TestStdBackend_VerifiesByDefault confirms the std net/http backend rejects
// a self-signed httptest TLS certificate by default (secure-by-default).
func TestStdBackend_VerifiesByDefault(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithStdHTTP())
	if err != nil {
		t.Fatal(err)
	}

	_, _, _, err = client.Do("GET", server.URL, nil, nil)
	if err == nil {
		t.Fatal("expected TLS verification error for self-signed cert, got nil")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "certificate") && !strings.Contains(low, "x509") && !strings.Contains(low, "authority") {
		t.Fatalf("expected error mentioning certificate/x509/authority, got: %v", err)
	}
}

// TestStdBackend_SkipVerifyOptIn confirms WithInsecureSkipVerify opts in to
// bypassing self-signed cert verification on the std net/http backend.
func TestStdBackend_SkipVerifyOptIn(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	client, err := NewClient(WithoutSSRFGuard(), WithStdHTTP(), WithInsecureSkipVerify())
	if err != nil {
		t.Fatal(err)
	}

	_, _, status, err := client.Do("GET", server.URL, nil, nil)
	if err != nil {
		t.Fatalf("expected success with skip-verify, got error: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}
}
