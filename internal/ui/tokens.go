package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/source"
)

// Tokens per chat, shown small left of the time. A chat's tokens are input +
// output + cache writes (cache re-reads are left out: they repeat the same
// context on every reply). Counting reads the whole log, so it runs in the
// background after a scan and is kept in <cache>/tokens.json by size and time:
// only chats that changed are read again.

type tokCount struct {
	Updated time.Time `json:"updated"`
	Size    int64     `json:"size"`
	N       int       `json:"n"`
}

type tokensMsg map[string]tokCount

func tokensFile() string { return filepath.Join(paths.CacheDir(), "tokens.json") }

func countTokens(items []catalog.Item, known map[string]tokCount) tea.Cmd {
	return func() tea.Msg {
		if known == nil {
			known = map[string]tokCount{}
			if b, err := os.ReadFile(tokensFile()); err == nil {
				_ = json.Unmarshal(b, &known)
			}
		}
		out := make(map[string]tokCount, len(items))
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4)
		for _, it := range items {
			k := it.Key()
			if c, ok := known[k]; ok && c.Size == it.Size && c.Updated.Equal(it.Updated) {
				out[k] = c
				continue
			}
			ur, ok := catalog.Adapter(it.Tool).(source.UsageReader)
			if !ok {
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(it catalog.Item) {
				defer func() { <-sem; wg.Done() }()
				n := 0
				for _, u := range ur.Usage(it.Session) {
					n += u.In + u.Out + u.CacheNew
				}
				mu.Lock()
				out[it.Key()] = tokCount{it.Updated, it.Size, n}
				mu.Unlock()
			}(it)
		}
		wg.Wait()
		if b, err := json.Marshal(out); err == nil {
			_ = os.MkdirAll(paths.CacheDir(), 0o755)
			_ = os.WriteFile(tokensFile(), b, 0o644)
		}
		return tokensMsg(out)
	}
}

// tokShort: 940, 12k, 1.4M.
func tokShort(n int) string {
	switch {
	case n >= 10_000_000:
		return fmt.Sprintf("%dM", n/1_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprint(n)
}
