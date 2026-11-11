package launch

import (
	"os/exec"
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
