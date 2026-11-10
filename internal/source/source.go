// Package source defines what a session is and how a tool adapter exposes them.
// Each AI tool (Claude Code, Codex, ...) lives in its own subpackage and only
// READS that tool's files; nothing here ever writes into another tool's data.
package source

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Session is one resumable conversation of one tool.
type Session struct {
	Tool    string    `json:"tool"`
	ID      string    `json:"id"`
	Title   string    `json:"title,omitempty"`  // tool-generated or user title
	Prompt  string    `json:"prompt,omitempty"` // first real user prompt
	CWD     string    `json:"cwd,omitempty"`
	Branch  string    `json:"branch,omitempty"`
	Updated time.Time `json:"updated"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
}

// Key identifies a session across tools.
func (s Session) Key() string { return s.Tool + ":" + s.ID }

// Label is the best human name available.
func (s Session) Label() string {
	if s.Title != "" {
		return s.Title
	}
	if s.Prompt != "" {
		return s.Prompt
	}
	return s.ID
}

// Adapter is implemented once per tool.
type Adapter interface {
	Name() string                     // "claude", "codex"
	Binary() string                   // executable to launch
	Available() bool                  // tool data exists on this machine
	List(c *Cache) ([]Session, error) // scan sessions, using c to skip unchanged files
	Resume(s Session) *exec.Cmd       // command that continues s
	New(dir string) *exec.Cmd         // command that starts a fresh session in dir
	Delete(s Session) error           // remove the session's own files (user-confirmed only)
	Preview(s Session, n int) []Turn  // the last n messages, read from the end of the file
}

// Turn is one message in a preview.
type Turn struct {
	User bool   // typed by the user (else the assistant)
	Text string // one line, cleaned
}

// LastLines reads complete lines from the end of the file at path (at most max bytes).
func LastLines(path string, max int) [][]byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	if st.Size() <= int64(max) {
		return HeadLines(f, max)
	}
	return TailLines(f, st.Size(), max)
}

// KeepLast keeps the last n turns.
func KeepLast(t []Turn, n int) []Turn {
	if len(t) > n {
		return t[len(t)-n:]
	}
	return t
}

// CleanText collapses whitespace and trims to max runes.
func CleanText(s string, max int) string {
	var b strings.Builder
	space := false
	n := 0
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
			n++
		}
		space = false
		b.WriteRune(r)
		n++
		if n >= max {
			b.WriteString("…")
			break
		}
	}
	return b.String()
}

// IsNoise reports prompts that are machine-injected rather than typed by the user.
func IsNoise(s string) bool {
	t := strings.TrimSpace(s)
	return t == "" || strings.HasPrefix(t, "<") || strings.HasPrefix(t, "Caveat:") || strings.HasPrefix(t, "[Request interrupted")
}

// RemoveUnder deletes each path, refusing anything that is not strictly inside
// root — the only way an adapter may write to another tool's data.
func RemoveUnder(root string, paths ...string) error {
	r, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	for _, p := range paths {
		a, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(r, a)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("refusing to delete %s: outside %s", p, root)
		}
		if err := os.RemoveAll(a); err != nil {
			return err
		}
	}
	return nil
}

// Transcriber is implemented by adapters that can read a whole chat (export).
type Transcriber interface {
	Transcript(s Session) ([]Turn, error)
}

// Usage is the token use of one assistant reply.
type Usage struct {
	Time                         time.Time
	Model                        string
	In, Out, CacheRead, CacheNew int
}

// UsageReader is implemented by adapters whose logs record token use.
type UsageReader interface {
	Usage(s Session) []Usage
}

// EachLine calls fn for every line of the file at path, however long.
func EachLine(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 256<<10)
	for {
		b, err := r.ReadBytes('\n')
		if len(b) > 0 {
			fn(b)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
