package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jedkx/jedkx-cli/internal/source"
)

func TestList(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "projects", "D--work")
	if err := os.MkdirAll(filepath.Join(proj, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"mode","sessionId":"abc"}`,
		`{"type":"user","isMeta":true,"cwd":"D:\\work","gitBranch":"main","message":{"role":"user","content":"<command-name>/clear</command-name>"}}`,
		`{"type":"user","cwd":"D:\\work","gitBranch":"main","message":{"role":"user","content":[{"type":"text","text":"  fix   the\nlogin bug  "}]}}`,
		`{"type":"ai-title","aiTitle":"Login bug fix"}`,
		`{"type":"assistant","gitBranch":"feature/x","message":{"role":"assistant","content":[]}}`,
	}
	write := func(p string, body string) {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(proj, "abc.jsonl"), strings.Join(lines, "\n")+"\n")
	write(filepath.Join(proj, "empty.jsonl"), `{"type":"mode"}`+"\n")
	write(filepath.Join(proj, "sub", "agent.jsonl"), lines[2]+"\n") // subagent transcript: ignored

	t.Setenv("CLAUDE_CONFIG_DIR", root)
	a := New()
	if !a.Available() {
		t.Fatal("adapter should be available")
	}
	c := source.LoadCache(filepath.Join(root, "cache.json"))
	got, err := a.List(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 session, got %d: %+v", len(got), got)
	}
	s := got[0]
	if s.ID != "abc" || s.Title != "Login bug fix" || s.Prompt != "fix the login bug" || s.CWD != `D:\work` || s.Branch != "feature/x" {
		t.Fatalf("parsed wrong: %+v", s)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	// second pass comes from cache and must match
	again, _ := a.List(source.LoadCache(filepath.Join(root, "cache.json")))
	if len(again) != 1 || again[0].Title != s.Title {
		t.Fatalf("cache roundtrip: %+v", again)
	}
	if cmd := a.Resume(s); strings.Join(cmd.Args, " ") != "claude --resume abc" {
		t.Fatalf("resume args: %v", cmd.Args)
	}
}

func TestPreview(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	body := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"first question"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use"},{"type":"text","text":"an answer"}]}}`,
		`{"type":"user","isSidechain":true,"message":{"role":"user","content":"agent noise"}}`,
		`{"type":"user","message":{"role":"user","content":"second question"}}`,
	}, "\n") + "\n"
	_ = os.WriteFile(p, []byte(body), 0o600)
	got := New().Preview(source.Session{Path: p}, 2)
	if len(got) != 2 || got[0].Text != "an answer" || got[0].User || got[1].Text != "second question" || !got[1].User {
		t.Fatalf("%+v", got)
	}
}

func TestTranscriptAndUsage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	body := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"line one\nline two"}}`,
		`{"type":"assistant","timestamp":"2026-11-10T10:00:00Z","message":{"id":"m1","model":"claude-x","role":"assistant","content":[{"type":"text","text":"answer"}],"usage":{"input_tokens":3,"output_tokens":7,"cache_read_input_tokens":100}}}`,
		`{"type":"assistant","timestamp":"2026-11-10T10:00:01Z","message":{"id":"m1","model":"claude-x","role":"assistant","content":[{"type":"tool_use"}],"usage":{"input_tokens":3,"output_tokens":7}}}`,
	}, "\n") + "\n"
	_ = os.WriteFile(p, []byte(body), 0o600)
	tr, err := New().Transcript(source.Session{Path: p})
	if err != nil || len(tr) != 2 || tr[0].Text != "line one\nline two" || tr[1].Text != "answer" {
		t.Fatalf("%+v %v", tr, err)
	}
	u := New().Usage(source.Session{Path: p})
	if len(u) != 1 || u[0].Out != 7 || u[0].CacheRead != 100 {
		t.Fatalf("usage counted twice or wrong: %+v", u)
	}
}
