// Package clean finds what Claude Code, Codex and jedkx-cli keep on disk that
// can go (caches, snapshots, file history) and removes it — only the folders
// listed here, only after you say yes. Chats are not in this list: old chats go
// through their adapter's Delete (jedkx-cli clean --older).
package clean

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jedkx/jedkx-cli/internal/paths"
)

// Spot is one folder whose contents can be removed.
type Spot struct {
	Tool, Name, What, Dir string
	Size                  int64
	Files                 int
}

func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(paths.Home(), ".claude")
}

func codexDir() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	return filepath.Join(paths.Home(), ".codex")
}

// Spots lists the removable folders that exist, with their size. Not here on
// purpose: plugins/cache (installed plugins run from it), sessions, settings.
func Spots() []Spot {
	c, x := claudeDir(), codexDir()
	all := []Spot{
		{Tool: "claude", Name: "file-history", What: "file versions kept for /rewind", Dir: filepath.Join(c, "file-history")},
		{Tool: "claude", Name: "shell-snapshots", What: "shell environment snapshots", Dir: filepath.Join(c, "shell-snapshots")},
		{Tool: "claude", Name: "paste-cache", What: "pasted text and images", Dir: filepath.Join(c, "paste-cache")},
		{Tool: "claude", Name: "todos", What: "old task lists", Dir: filepath.Join(c, "todos")},
		{Tool: "claude", Name: "debug", What: "debug logs", Dir: filepath.Join(c, "debug")},
		{Tool: "claude", Name: "cache", What: "Claude Code's cache", Dir: filepath.Join(c, "cache")},
		{Tool: "codex", Name: "cache", What: "Codex's cache", Dir: filepath.Join(x, "cache")},
		{Tool: "codex", Name: "tmp", What: "temporary files", Dir: filepath.Join(x, "tmp")},
		{Tool: "jedkx-cli", Name: "scan cache", What: "jedkx-cli's index (rebuilt on next start)", Dir: paths.CacheDir()},
	}
	var out []Spot
	for _, s := range all {
		if st, err := os.Stat(s.Dir); err != nil || !st.IsDir() {
			continue
		}
		s.Size, s.Files = size(s.Dir)
		if s.Files > 0 {
			out = append(out, s)
		}
	}
	return out
}

func size(dir string) (int64, int) {
	var n int64
	files := 0
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if i, e := d.Info(); e == nil {
				n += i.Size()
				files++
			}
		}
		return nil
	})
	return n, files
}

// Empty removes what is inside s.Dir (the folder itself stays). Files in use
// are skipped; it returns how much was freed.
func Empty(s Spot) (freed int64, err error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		p := filepath.Join(s.Dir, e.Name())
		n, _ := size(p)
		if !e.IsDir() {
			if i, e2 := e.Info(); e2 == nil {
				n = i.Size()
			}
		}
		if os.RemoveAll(p) == nil {
			freed += n
		}
	}
	return freed, nil
}
