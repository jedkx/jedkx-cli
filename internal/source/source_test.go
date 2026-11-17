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

func TestUnpasteKeepsPastedPrompt(t *testing.T) {
	in := "\n\n<pasted_content id=\"7c4c\">\nBuild a mail extension\n</pasted_content>\n"
	if IsNoise(Unpaste(in)) {
		t.Fatal("pasted prompt treated as noise")
	}
	if got := CleanText(Unpaste(in), 200); got != "Build a mail extension" {
		t.Fatalf("got %q", got)
	}
	if !IsNoise(Unpaste("<system-reminder>x</system-reminder>")) {
		t.Fatal("injected tag no longer noise")
	}
}
