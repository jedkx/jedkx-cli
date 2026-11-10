package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	sk := filepath.Join(home, ".claude", "skills", "commit")
	_ = os.MkdirAll(sk, 0o755)
	_ = os.WriteFile(filepath.Join(sk, "SKILL.md"), []byte("---\nname: commit\ndescription: \"writes the commit message\"\n---\nbody"), 0o600)
	proj := t.TempDir()
	_ = os.WriteFile(filepath.Join(proj, "CLAUDE.md"), []byte("\n# Project rules\n"), 0o600)
	got := Scan([]string{proj, proj})
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].Kind != "skill" || got[0].Name != "commit" || got[0].Desc != "writes the commit message" || got[0].Scope != "global" {
		t.Fatalf("skill: %+v", got[0])
	}
	if got[1].Kind != "instructions" || got[1].Desc != "Project rules" {
		t.Fatalf("project file: %+v", got[1])
	}
}
