package launch

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestOwnedWrapsCommand(t *testing.T) {
	c := exec.Command("claude", "--resume", "abc")
	c.Dir = "D:/x"
	w := owned(c, "/bin/jedkx-cli")
	want := []string{"/bin/jedkx-cli", PaneCommand, strconv.Itoa(os.Getpid()), "--", "claude", "--resume", "abc"}
	if len(w.Args) != len(want) || w.Dir != "D:/x" {
		t.Fatalf("got %v dir %q", w.Args, w.Dir)
	}
	for i := range want {
		if w.Args[i] != want[i] {
			t.Fatalf("arg %d = %q, want %q", i, w.Args[i], want[i])
		}
	}
}

func TestAlive(t *testing.T) {
	if !alive(os.Getpid()) {
		t.Fatal("this process must be alive")
	}
	if alive(1 << 30) {
		t.Fatal("a pid that cannot exist must not be alive")
	}
}

func TestRunPaneBadArgs(t *testing.T) {
	if RunPane([]string{"x"}) != 2 || RunPane([]string{"nan", "--", "echo"}) != 2 {
		t.Fatal("bad args must return 2")
	}
}
