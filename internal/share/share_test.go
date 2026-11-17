package share

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHome points home + config at a temp dir with both tools present.
func fakeHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("APPDATA", filepath.Join(h, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("CODEX_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	for _, d := range []string{".claude", ".codex"} {
		if err := os.MkdirAll(filepath.Join(h, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	look, run := lookTool, runTool
	lookTool = func(string) (string, error) { return "", errors.New("not on PATH") }
	t.Cleanup(func() { lookTool, runTool = look, run })
	return h
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSkillsShareBothWaysAndUndo(t *testing.T) {
	h := fakeHome(t)
	write(t, filepath.Join(h, ".codex", "skills", "audit", "SKILL.md"), "codex audit")
	write(t, filepath.Join(h, ".claude", "skills", "draft", "SKILL.md"), "claude draft")
	write(t, filepath.Join(h, ".claude", "skills", "same", "SKILL.md"), "x")
	write(t, filepath.Join(h, ".codex", "skills", "same", "SKILL.md"), "x")

	if err := Apply("jedkx-cli", Parts{Skills: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{".claude", ".codex"} {
		for _, n := range []string{"audit", "draft", "same"} {
			if _, err := os.Stat(filepath.Join(h, tool, "skills", n, "SKILL.md")); err != nil {
				t.Fatalf("%s does not see %s: %v", tool, n, err)
			}
		}
	}
	// a change made through one tool shows in the other
	write(t, filepath.Join(h, ".claude", "skills", "new", "SKILL.md"), "made in claude")
	if got := read(t, filepath.Join(h, ".codex", "skills", "new", "SKILL.md")); got != "made in claude" {
		t.Fatalf("codex sees %q", got)
	}
	if !Status()[0].On {
		t.Fatalf("status: %+v", Status()[0])
	}

	if err := UndoOnly(io.Discard, Parts{Skills: true}); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{".claude", ".codex"} {
		dir := filepath.Join(h, tool, "skills")
		if isLink(dir) {
			t.Fatalf("%s still a link", dir)
		}
		if got := read(t, filepath.Join(dir, "new", "SKILL.md")); got != "made in claude" {
			t.Fatalf("%s lost the shared skill: %q", tool, got)
		}
	}
	if Applied() {
		t.Fatal("state left behind")
	}
}

func TestSkillsConflictChangesNothing(t *testing.T) {
	h := fakeHome(t)
	write(t, filepath.Join(h, ".claude", "skills", "dup", "SKILL.md"), "one")
	write(t, filepath.Join(h, ".codex", "skills", "dup", "SKILL.md"), "two")
	err := Apply("jedkx-cli", Parts{Skills: true}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "dup") {
		t.Fatalf("want conflict error, got %v", err)
	}
	for _, tool := range []string{".claude", ".codex"} {
		if isLink(filepath.Join(h, tool, "skills")) {
			t.Fatal("linked despite the conflict")
		}
	}
	if got := read(t, filepath.Join(h, ".claude", "skills", "dup", "SKILL.md")); got != "one" {
		t.Fatalf("claude copy changed: %q", got)
	}
}

func TestInstructionsMergeAndUndo(t *testing.T) {
	h := fakeHome(t)
	codex := filepath.Join(h, ".codex", "AGENTS.md")
	claude := filepath.Join(h, ".claude", "CLAUDE.md")
	write(t, codex, "codex rules\n")
	write(t, claude, "claude rules\n")

	if err := Apply("jedkx-cli", Parts{Instructions: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	shared := read(t, AgentsFile())
	if !strings.Contains(shared, "codex rules") || !strings.Contains(shared, "claude rules") {
		t.Fatalf("shared file lost text: %q", shared)
	}
	if read(t, codex) != shared {
		t.Fatal("codex does not read the shared file")
	}
	md := read(t, claude)
	if !strings.Contains(md, "@"+filepath.ToSlash(AgentsFile())) || strings.Contains(md, "claude rules") {
		t.Fatalf("CLAUDE.md should only import the shared file: %q", md)
	}
	// editing the shared file reaches Codex (same file)
	if err := writeInPlace(AgentsFile(), "edited once\n"); err != nil {
		t.Fatal(err)
	}
	if read(t, codex) != "edited once\n" {
		t.Fatal("codex did not see the edit")
	}
	if !Status()[1].On {
		t.Fatalf("status: %+v", Status()[1])
	}
	// applying twice changes nothing
	if err := Apply("jedkx-cli", Parts{Instructions: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.Count(read(t, claude), blockStart) != 1 {
		t.Fatal("import line added twice")
	}

	if err := UndoOnly(io.Discard, Parts{Instructions: true}); err != nil {
		t.Fatal(err)
	}
	if isLink(codex) || read(t, codex) != "edited once\n" {
		t.Fatalf("codex AGENTS.md not given back: %q", read(t, codex))
	}
	if got := read(t, claude); strings.Contains(got, blockStart) || !strings.Contains(got, "edited once") {
		t.Fatalf("CLAUDE.md not given back: %q", got)
	}
	// on again after undo: the text is not doubled in CLAUDE.md
	if err := Apply("jedkx-cli", Parts{Instructions: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := read(t, claude); strings.Contains(got, "edited once") {
		t.Fatalf("shared text copied into CLAUDE.md again: %q", got)
	}
}

func TestInstructionsNewClaudeMDRemovedOnUndo(t *testing.T) {
	h := fakeHome(t)
	write(t, filepath.Join(h, ".codex", "AGENTS.md"), "")
	if err := Apply("jedkx-cli", Parts{Instructions: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := UndoOnly(io.Discard, Parts{Instructions: true}); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(h, ".claude", "CLAUDE.md")) {
		t.Fatal("an empty CLAUDE.md we created was left behind")
	}
}

func TestMCPUsesToolCommands(t *testing.T) {
	h := fakeHome(t)
	var ran []string
	lookTool = func(name string) (string, error) { return name, nil }
	runTool = func(name string, args ...string) error {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return nil
	}
	if err := Apply(filepath.Join(h, "jedkx-cli.exe"), Parts{MCP: true}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 2 || !strings.HasPrefix(ran[0], "claude mcp add --scope user jedkx-cli -- ") || !strings.HasPrefix(ran[1], "codex mcp add jedkx-cli -- ") ||
		!strings.HasSuffix(ran[1], " _mcp") {
		t.Fatalf("ran %q", ran)
	}
	ran = nil
	if err := UndoOnly(io.Discard, Parts{MCP: true}); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 2 {
		t.Fatalf("undo ran %q", ran)
	}
	if Applied() {
		t.Fatal("state left behind")
	}
}

func TestNeedsBothTools(t *testing.T) {
	h := fakeHome(t)
	if err := os.RemoveAll(filepath.Join(h, ".codex")); err != nil {
		t.Fatal(err)
	}
	if Available() || Apply("x", All, io.Discard) == nil {
		t.Fatal("share must refuse without Codex")
	}
}

func TestParseParts(t *testing.T) {
	p, err := ParseParts("skills,md")
	if err != nil || !p.Skills || !p.Instructions || p.MCP {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := ParseParts("themes"); err == nil {
		t.Fatal("want error")
	}
}
