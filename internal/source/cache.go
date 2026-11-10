package source

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/jedkx/jedkx-cli/internal/config"
)

// cacheVersion bumps whenever parsing changes, so old entries are re-read.
const cacheVersion = 4

type cacheEntry struct {
	ModTime int64   `json:"m"`
	Size    int64   `json:"s"`
	Session Session `json:"v"`
}

type cacheFile struct {
	Version int                   `json:"version"`
	Entries map[string]cacheEntry `json:"entries"`
}

// Cache remembers parsed sessions by file path + mtime + size, so a rescan only
// opens files that changed. It is rebuildable and safe to delete at any time.
type Cache struct {
	mu    sync.Mutex
	path  string
	data  cacheFile
	seen  map[string]bool
	dirty bool
}

// LoadCache opens the cache file (missing or stale → empty cache).
func LoadCache(path string) *Cache {
	c := &Cache{path: path, seen: map[string]bool{}}
	c.data.Entries = map[string]cacheEntry{}
	if b, err := os.ReadFile(path); err == nil {
		var f cacheFile
		if json.Unmarshal(b, &f) == nil && f.Version == cacheVersion && f.Entries != nil {
			c.data = f
		}
	}
	c.data.Version = cacheVersion
	return c
}

// Get returns a cached session if the file is unchanged.
func (c *Cache) Get(path string, mod, size int64) (Session, bool) {
	if c == nil {
		return Session{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen[path] = true
	e, ok := c.data.Entries[path]
	if !ok || e.ModTime != mod || e.Size != size {
		return Session{}, false
	}
	return e.Session, true
}

// Put stores a freshly parsed session.
func (c *Cache) Put(path string, mod, size int64, s Session) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen[path] = true
	c.data.Entries[path] = cacheEntry{ModTime: mod, Size: size, Session: s}
	c.dirty = true
}

// Save drops entries for files that no longer exist and writes the cache.
func (c *Cache) Save() error {
	if c == nil || c.path == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for p := range c.data.Entries {
		if !c.seen[p] {
			delete(c.data.Entries, p)
			c.dirty = true
		}
	}
	if !c.dirty {
		return nil
	}
	b, err := json.Marshal(c.data)
	if err != nil {
		return err
	}
	c.dirty = false
	return config.WriteAtomic(c.path, b)
}
