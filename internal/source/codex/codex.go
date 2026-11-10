// Package codex reads Codex CLI sessions from ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl[.zst]
// and their names from ~/.codex/session_index.jsonl. Read-only.
package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/source"
)

const headBytes = 512 << 10

// Adapter implements source.Adapter for Codex.
type Adapter struct{ home string }

// New returns the adapter; CODEX_HOME is honoured like the CLI does.
func New() *Adapter {
	h := os.Getenv("CODEX_HOME")
	if h == "" {
		h = filepath.Join(paths.Home(), ".codex")
	}
	return &Adapter{home: h}
}

func (a *Adapter) Name() string   { return "codex" }
func (a *Adapter) Binary() string { return "codex" }

func (a *Adapter) Available() bool {
	st, err := os.Stat(filepath.Join(a.home, "sessions"))
	return err == nil && st.IsDir()
}

type indexEntry struct {
	ID         string    `json:"id"`
	ThreadName string    `json:"thread_name"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// names reads session_index.jsonl (last entry per id wins).
func (a *Adapter) names() map[string]indexEntry {
	out := map[string]indexEntry{}
	f, err := os.Open(filepath.Join(a.home, "session_index.jsonl"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e indexEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.ID != "" {
			out[e.ID] = e
		}
	}
	return out
}

func (a *Adapter) List(c *source.Cache) ([]source.Session, error) {
	idx := a.names()
	var out []source.Session
	root := filepath.Join(a.home, "sessions")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "rollout-") || !(strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl.zst")) {
			return nil
		}
		st, err := d.Info()
		if err != nil || st.Size() == 0 {
			return nil
		}
		mod, size := st.ModTime().UnixNano(), st.Size()
		s, ok := c.Get(p, mod, size)
		if !ok {
			s = parse(p, st)
			c.Put(p, mod, size, s)
		}
		if s.ID == "" {
			return nil
		}
		if e, ok := idx[s.ID]; ok { // names can change without the rollout changing
			if e.ThreadName != "" {
				s.Title = source.CleanText(e.ThreadName, 120)
			}
			if e.UpdatedAt.After(s.Updated) {
				s.Updated = e.UpdatedAt
			}
		}
		if s.Prompt == "" && s.Title == "" {
			return nil // no real conversation (same rule as Claude)
		}
		out = append(out, s)
		return nil
	})
	return out, err
}

type rollLine struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type meta struct {
	ID  string `json:"id"`
	CWD string `json:"cwd"`
	Git *struct {
		Branch string `json:"branch"`
	} `json:"git"`
}

type msg struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

var kUser = []byte(`"role":"user"`)

// noise is text that is not part of the conversation: empty, tool wrappers, or
// (user turns only) the "# AGENTS.md …" instructions Codex injects; a reply may
// start with a markdown heading.
func noise(t string, user bool) bool {
	return t == "" || source.IsNoise(t) || (user && strings.HasPrefix(strings.TrimSpace(t), "#"))
}

func parse(path string, st os.FileInfo) source.Session {
	f, err := os.Open(path)
	if err != nil {
		return source.Session{}
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".zst") {
		d, err := zstd.NewReader(f, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true))
		if err != nil {
			return source.Session{}
		}
		defer d.Close()
		r = d
	}
	s := source.Session{Tool: "codex", Updated: st.ModTime(), Path: path, Size: st.Size()}
	for _, b := range source.HeadLines(r, headBytes) {
		if s.ID != "" && s.Prompt != "" {
			break
		}
		var l rollLine
		if json.Unmarshal(b, &l) != nil {
			continue
		}
		switch {
		case l.Type == "session_meta" && s.ID == "":
			var m meta
			if json.Unmarshal(l.Payload, &m) == nil {
				s.ID, s.CWD = m.ID, m.CWD
				if m.Git != nil {
					s.Branch = m.Git.Branch
				}
			}
		case l.Type == "response_item" && s.Prompt == "" && bytes.Contains(l.Payload, kUser):
			var m msg
			if json.Unmarshal(l.Payload, &m) == nil && m.Role == "user" {
				for _, c := range m.Content {
					if !noise(c.Text, true) {
						s.Prompt = source.CleanText(c.Text, 200)
						break
					}
				}
			}
		}
	}
	if s.ID == "" {
		s.ID = idFromName(filepath.Base(path))
	}
	return s
}

// idFromName takes the trailing UUID of rollout-<ts>-<uuid>.jsonl[.zst].
func idFromName(n string) string {
	n = strings.TrimSuffix(strings.TrimSuffix(n, ".zst"), ".jsonl")
	if len(n) >= 36 {
		return n[len(n)-36:]
	}
	return ""
}

// Preview returns the last n user/assistant messages that carry text.
func (a *Adapter) Preview(s source.Session, n int) []source.Turn {
	if strings.HasSuffix(s.Path, ".zst") {
		return nil // compressed archive: not worth decoding for a glance
	}
	var out []source.Turn
	for _, b := range source.LastLines(s.Path, 256<<10) {
		var l rollLine
		if json.Unmarshal(b, &l) != nil || l.Type != "response_item" {
			continue
		}
		var m msg
		if json.Unmarshal(l.Payload, &m) != nil || (m.Role != "user" && m.Role != "assistant") {
			continue
		}
		for _, c := range m.Content {
			if !noise(c.Text, m.Role == "user") {
				out = append(out, source.Turn{User: m.Role == "user", Text: source.CleanText(c.Text, 200)})
				break
			}
		}
	}
	return source.KeepLast(out, n)
}

func (a *Adapter) Resume(s source.Session) *exec.Cmd {
	cmd := exec.Command(a.Binary(), "resume", s.ID)
	cmd.Dir = existingDir(s.CWD)
	return cmd
}

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

// Delete removes the rollout file. The thread name in session_index.jsonl is left
// alone (Codex owns that file); without a rollout it is never listed.
func (a *Adapter) Delete(s source.Session) error {
	return source.RemoveUnder(filepath.Join(a.home, "sessions"), s.Path)
}

// Transcript reads the whole chat: every user and assistant message with text.
func (a *Adapter) Transcript(s source.Session) ([]source.Turn, error) {
	if strings.HasSuffix(s.Path, ".zst") {
		return nil, errors.New("compressed archive — open it in Codex to read it")
	}
	var out []source.Turn
	err := source.EachLine(s.Path, func(b []byte) {
		var l rollLine
		if json.Unmarshal(b, &l) != nil || l.Type != "response_item" {
			return
		}
		var m msg
		if json.Unmarshal(l.Payload, &m) != nil || (m.Role != "user" && m.Role != "assistant") {
			return
		}
		var parts []string
		for _, c := range m.Content {
			if !noise(c.Text, m.Role == "user") {
				parts = append(parts, strings.TrimSpace(c.Text))
			}
		}
		if len(parts) > 0 {
			out = append(out, source.Turn{User: m.Role == "user", Text: strings.Join(parts, "\n\n")})
		}
	})
	return out, err
}
