// Package meta stores what the USER adds to sessions (name, pin, archive, tags).
// It lives in jedkx-cli's own config dir; tool session files are never modified.
package meta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/paths"
)

// Entry is the user's data for one session.
type Entry struct {
	Name     string   `json:"name,omitempty"`
	Pinned   bool     `json:"pinned,omitempty"`
	Archived bool     `json:"archived,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

func (e Entry) empty() bool { return e.Name == "" && !e.Pinned && !e.Archived && len(e.Tags) == 0 }

// Store maps session keys ("claude:<id>") to entries.
type Store struct {
	mu      sync.Mutex
	path    string
	Entries map[string]Entry `json:"sessions"`
}

// File is the default store path.
func File() string { return filepath.Join(paths.ConfigDir(), "sessions.json") }

// Load opens the store (missing file → empty).
func Load(path string) *Store {
	s := &Store{path: path, Entries: map[string]Entry{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, s)
		if s.Entries == nil {
			s.Entries = map[string]Entry{}
		}
	}
	return s
}

// Get returns the entry for key (zero value if none).
func (s *Store) Get(key string) Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Entries[key]
}

// Update changes one entry and saves immediately (small file, rare writes).
func (s *Store) Update(key string, fn func(*Entry)) error {
	return s.UpdateMany([]string{key}, fn)
}

// UpdateMany applies fn to every key and saves the file once.
func (s *Store) UpdateMany(keys []string, fn func(*Entry)) error {
	s.mu.Lock()
	for _, key := range keys {
		e := s.Entries[key]
		fn(&e)
		if e.empty() {
			delete(s.Entries, key)
		} else {
			s.Entries[key] = e
		}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return config.WriteAtomic(s.path, b)
}
