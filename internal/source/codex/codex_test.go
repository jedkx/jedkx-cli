package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"

	"github.com/jedkx/jedkx-cli/internal/source"
)

const id = "01a17234-748a-7750-b528-6c73e78de1ae"

func TestList(t *testing.T) {
	home := t.TempDir()
	day := filepath.Join(home, "sessions", "2026", "10", "25")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`{"type":"session_meta","payload":{"id":"` + id + `","cwd":"C:\\proj","git":{"branch":"dev"}}}`,
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"system"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>x</environment_context>"},{"type":"input_text","text":"write a script"}]}}`,
	}, "\n") + "\n"

	// compressed rollout, like recent Codex versions write
	f, err := os.Create(filepath.Join(day, "rollout-2026-10-25T11-36-42-"+id+".jsonl.zst"))
	if err != nil {
		t.Fatal(err)
	}
	w, _ := zstd.NewWriter(f)
	_, _ = w.Write([]byte(body))
	w.Close()
	f.Close()

	idx := `{"id":"` + id + `","thread_name":"Script task","updated_at":"2026-10-25T08:38:04Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(home, "session_index.jsonl"), []byte(idx), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CODEX_HOME", home)
	got, err := New().List(source.LoadCache(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	s := got[0]
	if s.ID != id || s.CWD != `C:\proj` || s.Branch != "dev" || s.Prompt != "write a script" || s.Title != "Script task" {
		t.Fatalf("parsed wrong: %+v", s)
	}
	if cmd := New().Resume(s); strings.Join(cmd.Args, " ") != "codex resume "+id {
		t.Fatalf("resume args: %v", cmd.Args)
	}
}

func TestIDFromName(t *testing.T) {
	if got := idFromName("rollout-2026-10-25T11-36-42-" + id + ".jsonl"); got != id {
		t.Fatal(got)
	}
}
