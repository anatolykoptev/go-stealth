package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	stealth "github.com/anatolykoptev/go-stealth"
)

// SessionStore persists and loads session state.
type SessionStore interface {
	Save(s *Session) error
	Load(id string) (*Session, error)
	List() ([]string, error)
	Delete(id string) error
}

// sessionData is the serializable form of a Session.
type sessionData struct {
	ID           string                `json:"id"`
	CreatedAt    time.Time             `json:"created_at"`
	LastUsed     time.Time             `json:"last_used"`
	RequestCount int64                 `json:"request_count"`
	Profile      stealth.BrowserProfile `json:"profile"`
}

func (s *Session) toData() sessionData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sessionData{
		ID:           s.ID,
		CreatedAt:    s.CreatedAt,
		LastUsed:     s.lastUsed,
		RequestCount: s.requestCount.Load(),
		Profile:      s.profile,
	}
}

// FileStore persists sessions as JSON files in a directory.
type FileStore struct {
	dir string
	mu  sync.Mutex
}

// NewFileStore creates a FileStore that saves sessions to the given directory.
// The directory is created if it doesn't exist.
func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	return &FileStore{dir: dir}, nil
}

func (fs *FileStore) path(id string) string {
	return filepath.Join(fs.dir, id+".json")
}

// Save persists a session to disk.
func (fs *FileStore) Save(s *Session) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data := s.toData()
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err := os.WriteFile(fs.path(s.ID), b, 0o600); err != nil {
		return fmt.Errorf("write session: %w", err)
	}
	return nil
}

// Load restores a session from disk. The returned session has a new BrowserClient
// with the saved profile but fresh cookies/TLS state.
func (fs *FileStore) Load(id string) (*Session, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	b, err := os.ReadFile(fs.path(id))
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}

	var data sessionData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	// Rebuild session with saved profile
	s, err := New(WithProfile(data.Profile))
	if err != nil {
		return nil, fmt.Errorf("rebuild session: %w", err)
	}
	s.ID = data.ID
	s.CreatedAt = data.CreatedAt
	s.lastUsed = data.LastUsed
	s.requestCount.Store(data.RequestCount)

	return s, nil
}

// List returns IDs of all saved sessions.
func (fs *FileStore) List() ([]string, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if ext := filepath.Ext(name); ext == ".json" {
			ids = append(ids, name[:len(name)-5])
		}
	}
	return ids, nil
}

// Delete removes a saved session.
func (fs *FileStore) Delete(id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := os.Remove(fs.path(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
