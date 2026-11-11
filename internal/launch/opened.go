package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/jedkx/jedkx-cli/internal/paths"
)

// Chats opened from the list (beside it or in a tab) leave a small marker while
// they run: <cache>/open/<key>.json with the wrapper's pid, the tab title and
// where it is. Enter on a chat that already runs focuses it instead of starting
// a second copy. Nothing polls: markers are read only when you open a chat, and
// one whose process is gone is dropped then. Chats started outside jedkx-cli
// leave no marker.

// Opened is a running chat that jedkx-cli started.
type Opened struct {
	PID   int    `json:"pid"`
	Title string `json:"title"`
	Where string `json:"where"` // "pane" or "tab"
}

func openDir() string { return filepath.Join(paths.CacheDir(), "open") }

// markerFile maps a session key ("claude:abc") to its marker file.
func markerFile(key string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '-' || r == '.' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '_'
	}, key)
	return filepath.Join(openDir(), safe+".json")
}

// IsOpen reports whether the chat with this key runs in a pane or tab that
// jedkx-cli opened.
func IsOpen(key string) (Opened, bool) {
	if key == "" {
		return Opened{}, false
	}
	f := markerFile(key)
	b, err := os.ReadFile(f)
	if err != nil {
		return Opened{}, false
	}
	var o Opened
	if json.Unmarshal(b, &o) != nil || o.PID <= 0 || !alive(o.PID) {
		_ = os.Remove(f) // left behind by a closed window
		return Opened{}, false
	}
	return o, true
}

// retitle changes the title in a running chat's marker (the tab was renamed),
// so focusing the tab still finds it.
func retitle(key, title string) {
	f := markerFile(key)
	b, err := os.ReadFile(f)
	if err != nil {
		return
	}
	var o Opened
	if json.Unmarshal(b, &o) != nil {
		return
	}
	o.Title = title
	if b, err = json.Marshal(o); err == nil {
		_ = os.WriteFile(f, b, 0o600)
	}
}

// mark writes the marker for this process; the returned func removes it.
func mark(key, title, where string) func() {
	if key == "" {
		return func() {}
	}
	f := markerFile(key)
	b, _ := json.Marshal(Opened{PID: os.Getpid(), Title: title, Where: where})
	if os.MkdirAll(openDir(), 0o755) != nil || os.WriteFile(f, b, 0o600) != nil {
		return func() {}
	}
	return func() { _ = os.Remove(f) }
}
