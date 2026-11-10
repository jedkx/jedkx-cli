package setup

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jedkx/jedkx-cli/internal/paths"
)

// Backup copies every file setup can touch — Claude Code's settings.json and
// themes, Windows Terminal's settings.json — into a new folder under dir,
// keeping their paths below the home folder (dir/<backup>/.claude/settings.json,
// …), so each file can be copied straight back. It returns the backup folder.
func Backup(dir string, p Parts) (string, error) {
	root := filepath.Join(dir, "jedkx-cli-backup-"+time.Now().Format("20060102-150405"))
	var files []string
	if p.Claude {
		files = append(files, claudeSettings())
		if themes, _ := filepath.Glob(filepath.Join(claudeDir(), "themes", "*")); len(themes) > 0 {
			files = append(files, themes...)
		}
	}
	if p.Terminal {
		files = append(files, wtSettingsFiles()...)
	}
	home := paths.Home()
	n := 0
	for _, f := range files {
		rel, err := filepath.Rel(home, f)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = filepath.Join("other", filepath.Base(filepath.Dir(f)), filepath.Base(f))
		}
		copied, err := copyFile(f, filepath.Join(root, rel))
		switch {
		case errors.Is(err, os.ErrNotExist):
			continue // nothing to keep
		case err != nil:
			_ = os.RemoveAll(root) // no half backup left behind
			return "", err
		}
		if copied {
			n++
		}
	}
	if n == 0 {
		return "", nil
	}
	return root, nil
}

// copyFile copies one regular file; folders are skipped (copied = false).
func copyFile(from, to string) (copied bool, err error) {
	st, err := os.Stat(from)
	if err != nil {
		return false, err
	}
	if st.IsDir() {
		return false, nil
	}
	in, err := os.Open(from)
	if err != nil {
		return false, err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return false, err
	}
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return false, err
	}
	return true, out.Close()
}
