// Package catalog is the registry of tool adapters and the merged session view.
// Adding a tool = one line in Adapters().
package catalog

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/source"
	"github.com/jedkx/jedkx-cli/internal/source/claude"
	"github.com/jedkx/jedkx-cli/internal/source/codex"
)

// Adapters lists every supported tool.
func Adapters() []source.Adapter {
	return []source.Adapter{claude.New(), codex.New()}
}

// Adapter finds one by name.
func Adapter(name string) source.Adapter {
	for _, a := range Adapters() {
		if a.Name() == name {
			return a
		}
	}
	return nil
}

// Item is a session plus the user's own data.
type Item struct {
	source.Session
	meta.Entry
}

// Label prefers the user's name.
func (i Item) Label() string {
	if i.Name != "" {
		return i.Name
	}
	return i.Session.Label()
}

// Project is a short name for the session's directory.
func (i Item) Project() string {
	if i.CWD == "" {
		return "-"
	}
	return filepath.Base(filepath.Clean(i.CWD))
}

// ProjectKey identifies the session's folder: the full cleaned path (case-
// insensitive on Windows), so D:pi and D:pi stay apart.
func (i Item) ProjectKey() string {
	if i.CWD == "" {
		return ""
	}
	p := filepath.Clean(i.CWD)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// Result of a full scan.
type Result struct {
	Items   []Item
	Tools   []string // tools found on this machine
	Missing []string // tools whose binary is not on PATH
	Took    time.Duration
}

// CacheFile is where the scan cache lives.
func CacheFile() string { return filepath.Join(paths.CacheDir(), "index.json") }

// Load scans all available tools in parallel and merges user metadata;
// tools named in hidden are skipped (the user chose not to list them).
func Load(store *meta.Store, hidden ...string) Result {
	start := time.Now()
	cache := source.LoadCache(CacheFile())
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		res Result
	)
	for _, a := range Adapters() {
		if !a.Available() || slices.Contains(hidden, a.Name()) {
			continue
		}
		res.Tools = append(res.Tools, a.Name())
		if _, err := exec.LookPath(a.Binary()); err != nil {
			res.Missing = append(res.Missing, a.Name())
		}
		wg.Add(1)
		go func(a source.Adapter) {
			defer wg.Done()
			list, _ := a.List(cache)
			items := make([]Item, 0, len(list))
			for _, s := range list {
				items = append(items, Item{Session: s, Entry: store.Get(s.Key())})
			}
			mu.Lock()
			res.Items = append(res.Items, items...)
			mu.Unlock()
		}(a)
	}
	wg.Wait()
	_ = cache.Save()
	Sort(res.Items)
	res.Took = time.Since(start)
	return res
}

// Sort: pinned first, then most recent.
func Sort(items []Item) {
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].Pinned != items[b].Pinned {
			return items[a].Pinned
		}
		return items[a].Updated.After(items[b].Updated)
	})
}

// Matches is a case- and Turkish-insensitive search over the visible fields.
func Matches(i Item, q string) bool {
	if q == "" {
		return true
	}
	hay := Fold(strings.Join([]string{i.Label(), i.Prompt, i.Project(), i.CWD, i.Branch, i.Tool, i.ID, strings.Join(i.Tags, " ")}, " "))
	for _, w := range strings.Fields(Fold(q)) {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

var foldMap = strings.NewReplacer("İ", "i", "I", "i", "ı", "i", "Ş", "s", "ş", "s", "Ğ", "g", "ğ", "g", "Ü", "u", "ü", "u", "Ö", "o", "ö", "o", "Ç", "c", "ç", "c")

// Fold lowercases and strips Turkish diacritics so "islem" finds "İŞLEM".
func Fold(s string) string { return strings.ToLower(foldMap.Replace(s)) }
