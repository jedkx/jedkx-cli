// Package claude reads Claude Code sessions from ~/.claude/projects/<project>/<id>.jsonl.
// Read-only: files are opened, sampled (head + tail) and closed.
package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/source"
)

const (
	headBytes = 512 << 10
	tailBytes = 256 << 10
)

// Adapter implements source.Adapter for Claude Code.
type Adapter struct{ root string }

// New returns the adapter; CLAUDE_CONFIG_DIR is honoured like the CLI does.
func New() *Adapter {
	base := os.Getenv("CLAUDE_CONFIG_DIR")
	if base == "" {
		base = filepath.Join(paths.Home(), ".claude")
	}
	return &Adapter{root: filepath.Join(base, "projects")}
}

func (a *Adapter) Name() string   { return "claude" }
func (a *Adapter) Binary() string { return "claude" }

func (a *Adapter) Available() bool {
	st, err := os.Stat(a.root)
	return err == nil && st.IsDir()
}

// List scans top-level session files (subagent transcripts in subfolders are skipped).
func (a *Adapter) List(c *source.Cache) ([]source.Session, error) {
	files, err := filepath.Glob(filepath.Join(a.root, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	out := make([]source.Session, 0, len(files))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // bounded: light on disk and CPU
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil || st.Size() == 0 {
			continue
		}
		mod, size := st.ModTime().UnixNano(), st.Size()
		if s, ok := c.Get(f, mod, size); ok {
			if s.ID != "" {
				mu.Lock()
				out = append(out, s)
				mu.Unlock()
			}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(f string, st os.FileInfo) {
			defer wg.Done()
			defer func() { <-sem }()
			s := parse(f, st)
			c.Put(f, st.ModTime().UnixNano(), st.Size(), s) // empty sessions cached too, so they are not re-read
			if s.ID != "" {
				mu.Lock()
				out = append(out, s)
				mu.Unlock()
			}
		}(f, st)
	}
	wg.Wait()
	return out, nil
}

type line struct {
	Type        string          `json:"type"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	SessionID   string          `json:"sessionId"`
	AITitle     string          `json:"aiTitle"`
	CustomTitle string          `json:"customTitle"`
	IsSidechain bool            `json:"isSidechain"`
	IsMeta      bool            `json:"isMeta"`
	Message     json.RawMessage `json:"message"`
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

var (
	kUser   = []byte(`"type":"user"`)
	kTitle  = []byte(`-title"`)
	kBranch = []byte(`"gitBranch"`)
)

// parse samples the file: head for cwd/branch/first prompt, tail for the latest title/branch.
// ParseFile reads one chat file (title, prompt, folder, branch).
func ParseFile(path string) (source.Session, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return source.Session{}, false
	}
	s := parse(path, st)
	return s, s.ID != ""
}

func parse(path string, st os.FileInfo) source.Session {
	s := source.Session{
		Tool:    "claude",
		ID:      strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		Updated: st.ModTime(),
		Path:    path,
		Size:    st.Size(),
	}
	f, err := os.Open(path)
	if err != nil {
		return source.Session{}
	}
	defer f.Close()

	var aiTitle, customTitle string
	scan := func(lines [][]byte, head bool) {
		for _, b := range lines {
			if !bytes.Contains(b, kUser) && !bytes.Contains(b, kTitle) && !bytes.Contains(b, kBranch) {
				continue
			}
			var l line
			if json.Unmarshal(b, &l) != nil {
				continue
			}
			switch l.Type {
			case "ai-title":
				aiTitle = l.AITitle
			case "custom-title":
				customTitle = l.CustomTitle
			}
			if l.GitBranch != "" && l.GitBranch != "HEAD" { // in order: the last one seen is current; HEAD = no repo/detached
				s.Branch = l.GitBranch
			}
			if s.CWD == "" && l.CWD != "" {
				s.CWD = l.CWD
			}
			if head && s.Prompt == "" && l.Type == "user" && !l.IsSidechain && !l.IsMeta {
				s.Prompt = promptText(l.Message)
			}
		}
	}
	scan(source.HeadLines(f, headBytes), true)
	scan(source.TailLines(f, st.Size(), tailBytes), false)

	switch {
	case customTitle != "":
		s.Title = source.CleanText(customTitle, 120)
	case aiTitle != "":
		s.Title = source.CleanText(aiTitle, 120)
	}
	if s.Prompt == "" && s.Title == "" {
		return source.Session{} // no real conversation (e.g. aborted start)
	}
	return s
}

// promptText extracts typed text from a user message (string or content blocks).
func promptText(raw json.RawMessage) string {
	var m message
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	var str string
	if json.Unmarshal(m.Content, &str) == nil {
		if source.IsNoise(str) {
			return ""
		}
		return source.CleanText(str, 200)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && !source.IsNoise(b.Text) {
				return source.CleanText(b.Text, 200)
			}
		}
	}
	return ""
}

// Preview returns the last n user/assistant messages that carry text.
func (a *Adapter) Preview(s source.Session, n int) []source.Turn {
	var out []source.Turn
	for _, b := range source.LastLines(s.Path, tailBytes) {
		var l line
		if json.Unmarshal(b, &l) != nil || l.IsSidechain || l.IsMeta || (l.Type != "user" && l.Type != "assistant") {
			continue
		}
		if t := promptText(l.Message); t != "" {
			out = append(out, source.Turn{User: l.Type == "user", Text: t})
		}
	}
	return source.KeepLast(out, n)
}

// Resume continues a session in its original directory.
func (a *Adapter) Resume(s source.Session) *exec.Cmd {
	cmd := exec.Command(a.Binary(), "--resume", s.ID)
	cmd.Dir = existingDir(s.CWD)
	return cmd
}

// New starts a fresh session in dir.
func (a *Adapter) New(dir string) *exec.Cmd {
	cmd := exec.Command(a.Binary())
	cmd.Dir = existingDir(dir)
	return cmd
}

func existingDir(d string) string {
	if d == "" {
		return ""
	}
	if st, err := os.Stat(d); err == nil && st.IsDir() {
		return d
	}
	return ""
}

// Delete removes the transcript and its sidecar folder (subagents, tool results).
func (a *Adapter) Delete(s source.Session) error {
	paths := []string{s.Path}
	if side := strings.TrimSuffix(s.Path, ".jsonl"); side != s.Path {
		if st, err := os.Stat(side); err == nil && st.IsDir() {
			paths = append(paths, side)
		}
	}
	return source.RemoveUnder(a.root, paths...)
}

// rawText is the whole text of a message: the string content, or its text
// blocks joined; tool calls and results are left out. Noise gives "".
func rawText(raw json.RawMessage) string {
	var m message
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	var str string
	if json.Unmarshal(m.Content, &str) == nil {
		if source.IsNoise(str) {
			return ""
		}
		return strings.TrimSpace(str)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && !source.IsNoise(b.Text) {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, "\n\n")
}

// Transcript reads the whole chat: every user and assistant message with text.
func (a *Adapter) Transcript(s source.Session) ([]source.Turn, error) {
	var out []source.Turn
	err := source.EachLine(s.Path, func(b []byte) {
		if !bytes.Contains(b, []byte(`"type":"user"`)) && !bytes.Contains(b, []byte(`"type":"assistant"`)) {
			return
		}
		var l line
		if json.Unmarshal(b, &l) != nil || l.IsSidechain || l.IsMeta || (l.Type != "user" && l.Type != "assistant") {
			return
		}
		if t := rawText(l.Message); t != "" {
			out = append(out, source.Turn{User: l.Type == "user", Text: t})
		}
	})
	return out, err
}

// Usage reads the token use of every assistant reply. Claude writes one line per
// content block with the same message id; each reply is counted once.
func (a *Adapter) Usage(s source.Session) []source.Usage {
	var out []source.Usage
	seen := map[string]bool{}
	_ = source.EachLine(s.Path, func(b []byte) {
		if !bytes.Contains(b, []byte(`"usage"`)) || !bytes.Contains(b, []byte(`"type":"assistant"`)) {
			return
		}
		var l struct {
			Timestamp time.Time `json:"timestamp"`
			Message   struct {
				ID    string `json:"id"`
				Model string `json:"model"`
				Usage struct {
					In         int `json:"input_tokens"`
					Out        int `json:"output_tokens"`
					CacheRead  int `json:"cache_read_input_tokens"`
					CacheWrite int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(b, &l) != nil || l.Message.ID == "" || seen[l.Message.ID] {
			return
		}
		seen[l.Message.ID] = true
		u := l.Message.Usage
		out = append(out, source.Usage{Time: l.Timestamp, Model: l.Message.Model, In: u.In, Out: u.Out, CacheRead: u.CacheRead, CacheNew: u.CacheWrite})
	})
	return out
}
