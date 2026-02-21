package session

import (
	"os"
	"testing"
	"time"

	stealth "github.com/anatolykoptev/go-stealth"
)

func TestFileStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := New()
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.ID != s.ID {
		t.Fatalf("ID mismatch: %s vs %s", loaded.ID, s.ID)
	}
	if !loaded.CreatedAt.Equal(s.CreatedAt) {
		t.Fatalf("CreatedAt mismatch")
	}
	if loaded.Profile().UserAgent != s.Profile().UserAgent {
		t.Fatalf("profile UA mismatch: %s vs %s", loaded.Profile().UserAgent, s.Profile().UserAgent)
	}
	if loaded.Profile().Browser != s.Profile().Browser {
		t.Fatalf("profile browser mismatch")
	}
}

func TestFileStore_SaveLoad_WithProfile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	p := stealth.RandomProfile(stealth.WithBrowser("firefox"), stealth.WithOS("linux"))
	s, err := New(WithProfile(p))
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Profile().Browser != "firefox" {
		t.Fatalf("expected firefox, got %s", loaded.Profile().Browser)
	}
	if loaded.Profile().OS != "linux" {
		t.Fatalf("expected linux, got %s", loaded.Profile().OS)
	}
}

func TestFileStore_RequestCount(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// Simulate 42 requests
	s.requestCount.Store(42)

	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.RequestCount() != 42 {
		t.Fatalf("expected 42 requests, got %d", loaded.RequestCount())
	}
}

func TestFileStore_List(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s1, _ := New()
	s2, _ := New()
	store.Save(s1)
	store.Save(s2)

	ids, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(ids))
	}

	found := map[string]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found[s1.ID] || !found[s2.ID] {
		t.Fatalf("missing session IDs: got %v, want %s and %s", ids, s1.ID, s2.ID)
	}
}

func TestFileStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s, _ := New()
	store.Save(s)

	if err := store.Delete(s.ID); err != nil {
		t.Fatal(err)
	}

	_, err = store.Load(s.ID)
	if err == nil {
		t.Fatal("expected error loading deleted session")
	}

	ids, _ := store.List()
	if len(ids) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(ids))
	}
}

func TestFileStore_LoadNonexistent(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestFileStore_DeleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should not error on deleting nonexistent
	if err := store.Delete("nonexistent"); err != nil {
		t.Fatal(err)
	}
}

func TestFileStore_PreservesTimestamps(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s, _ := New()
	time.Sleep(5 * time.Millisecond)
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()

	store.Save(s)
	loaded, _ := store.Load(s.ID)

	// Age should be preserved (CreatedAt is earlier than lastUsed)
	if loaded.CreatedAt.After(loaded.LastUsed()) {
		t.Fatal("CreatedAt should be before LastUsed")
	}
}

func TestFileStore_CreatesDirIfMissing(t *testing.T) {
	dir := t.TempDir() + "/sub/deep"
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s, _ := New()
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("directory should exist")
	}
}
