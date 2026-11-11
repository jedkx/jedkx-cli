package clean

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSpotsAndEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "local"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	d := filepath.Join(home, ".claude", "file-history", "s1")
	_ = os.MkdirAll(d, 0o755)
	_ = os.WriteFile(filepath.Join(d, "a"), make([]byte, 100), 0o600)
	keep := filepath.Join(home, ".claude", "plugins", "cache", "p")
	_ = os.MkdirAll(keep, 0o755)
	_ = os.WriteFile(filepath.Join(keep, "x"), []byte("x"), 0o600)
	var fh *Spot
	for _, s := range Spots() {
		if s.Name == "file-history" {
			fh = &s
		}
		if s.Dir == filepath.Dir(keep) {
			t.Fatal("plugin cache must never be offered")
		}
	}
	if fh == nil || fh.Size != 100 {
		t.Fatalf("file-history not found: %+v", fh)
	}
	if n, err := Empty(*fh); err != nil || n != 100 {
		t.Fatal(n, err)
	}
	if _, err := os.Stat(fh.Dir); err != nil {
		t.Fatal("the folder itself must stay")
	}
}
