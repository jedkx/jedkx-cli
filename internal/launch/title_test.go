package launch

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestClaudeInside(t *testing.T) {
	cases := map[bool]*exec.Cmd{
		true:  exec.Command("jedkx-cli", "_pane", "0", "-", "tab", "x", "--", `C:\bin\claude.exe`, "--resume", "1"),
		false: exec.Command("jedkx-cli", "_pane", "0", "-", "tab", "x", "--", "codex", "resume", "1"),
	}
	for want, c := range cases {
		if got := claudeInside(c); got != want {
			t.Errorf("%v: got %v", c.Args, got)
		}
	}
	if !claudeInside(exec.Command("claude")) {
		t.Error("plain claude")
	}
}

func TestTabTitleOnlyInOurTabs(t *testing.T) {
	t.Setenv(TabKeyEnv, "")
	if tabTitle() != "" {
		t.Fatal("outside a jedkx-cli tab the title is Claude's")
	}
}

func TestClaimMovesTheMarker(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	claim("claude:old", os.Getpid(), "old chat", "tab")
	claim("claude:new", os.Getpid(), "new chat", "tab") // /clear in the same tab
	if _, err := os.Stat(markerFile("claude:old")); err == nil {
		t.Fatal("the chat that left the tab keeps a marker")
	}
	b, err := os.ReadFile(markerFile("claude:new"))
	if err != nil || !strings.Contains(string(b), "new chat") {
		t.Fatalf("new marker: %s %v", b, err)
	}
}
