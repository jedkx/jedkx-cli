package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveUnder(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "p", "s.jsonl")
	_ = os.MkdirAll(filepath.Dir(in), 0o700)
	_ = os.WriteFile(in, nil, 0o600)
	outside := filepath.Join(t.TempDir(), "keep.txt")
	_ = os.WriteFile(outside, nil, 0o600)
	for _, bad := range []string{outside, root, filepath.Join(root, "..", "x")} {
		if RemoveUnder(root, bad) == nil {
			t.Errorf("must refuse %s", bad)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside file was touched")
	}
	if err := RemoveUnder(root, in); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(in); !os.IsNotExist(err) {
		t.Fatal("session file not deleted")
	}
}
