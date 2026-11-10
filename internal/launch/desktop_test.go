package launch

import (
	"os/exec"
	"runtime"
	"testing"
)

func TestCommandLine(t *testing.T) {
	c := exec.Command("claude", "--resume", "abc")
	c.Dir = `C:\Users\x y\proj`
	want := `cd "C:\Users\x y\proj" && claude --resume abc`
	if runtime.GOOS == "windows" {
		want = `Set-Location "C:\Users\x y\proj"; claude --resume abc`
	}
	if got := CommandLine(c); got != want {
		t.Fatalf("got %s", got)
	}
}
