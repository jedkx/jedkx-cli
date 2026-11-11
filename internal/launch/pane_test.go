package launch

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
)

func TestWrapCommand(t *testing.T) {
	c := exec.Command("claude", "--resume", "abc")
	c.Dir = "D:/x"
	w := wrap(c, "/bin/jedkx-cli", os.Getpid(), "claude:abc", "pane", "Fix login")
	want := []string{"/bin/jedkx-cli", PaneCommand, strconv.Itoa(os.Getpid()), "claude:abc", "pane", "Fix login", "--", "claude", "--resume", "abc"}
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
	if RunPane([]string{"x"}) != 2 || RunPane([]string{"nan", "k", "tab", "t", "--", "echo"}) != 2 {
		t.Fatal("bad args must return 2")
	}
}

// A marker counts only while its process runs; a stale one is dropped.
func TestOpenedMarker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
	unmark := mark("claude:abc/../x", "Fix login", "tab")
	o, ok := IsOpen("claude:abc/../x")
	if !ok || o.Title != "Fix login" || o.Where != "tab" {
		t.Fatalf("marker not found: %+v %v", o, ok)
	}
	unmark()
	if _, ok := IsOpen("claude:abc/../x"); ok {
		t.Fatal("removed marker still open")
	}
	_ = os.MkdirAll(openDir(), 0o755)
	_ = os.WriteFile(markerFile("codex:z"), []byte(`{"pid":1073741824,"title":"x"}`), 0o600)
	if _, ok := IsOpen("codex:z"); ok {
		t.Fatal("a dead pid must not count")
	}
	if _, err := os.Stat(markerFile("codex:z")); !os.IsNotExist(err) {
		t.Fatal("stale marker not removed")
	}
}
