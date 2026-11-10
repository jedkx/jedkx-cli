package launch

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// Copy puts text on the system clipboard.
func Copy(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Set-Clipboard keeps non-ASCII paths intact (clip.exe would not)
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "$input | Set-Clipboard")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}} {
			if _, err := exec.LookPath(c[0]); err == nil {
				cmd = exec.Command(c[0], c[1:]...)
				break
			}
		}
		if cmd == nil {
			return errors.New("no clipboard tool (install wl-copy or xclip)")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// OpenFolder shows dir in the file manager.
func OpenFolder(dir string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	}
	return exec.Command("xdg-open", dir).Start()
}

// CommandLine is a copy-pasteable shell line for cmd: cd into its folder, run it.
// Windows gets PowerShell syntax (its default shell; "&&" fails in 5.1).
func CommandLine(cmd *exec.Cmd) string {
	q := func(s string) string {
		if strings.ContainsAny(s, " \t\"'&;|()<>") {
			return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
		}
		return s
	}
	parts := make([]string, len(cmd.Args))
	for i, a := range cmd.Args {
		parts[i] = q(a)
	}
	line := strings.Join(parts, " ")
	if cmd.Dir != "" {
		if runtime.GOOS == "windows" {
			line = "Set-Location " + q(cmd.Dir) + "; " + line
		} else {
			line = "cd " + q(cmd.Dir) + " && " + line
		}
	}
	return line
}
