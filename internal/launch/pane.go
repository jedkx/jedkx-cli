package launch

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Chats opened beside the list or in a tab run through `jedkx-cli _pane <owner
// pid> <key> <where> <title> -- <tool …>`, a small wrapper that runs the tool and
// leaves an "open" marker while it runs (opened.go). Panes belong to the list:
// the wrapper ends the tool once the list (the owner) is gone, so closing
// jedkx-cli closes the chats it opened beside it. Tabs are independent: owner 0,
// nothing is polled. The tool's own exit (ctrl+c, /exit) closes just that pane
// or tab.

// PaneCommand is the internal subcommand name used by the wrapper.
const PaneCommand = "_pane"

// noKey stands for "no session yet" (a new chat) in the wrapper's arguments.
const noKey = "-"

// ownerPoll is how often a pane's wrapper checks that the list is still running.
const ownerPoll = time.Second

// wrap rewrites cmd so it runs under the wrapper. owner 0 = not owned (a tab).
func wrap(cmd *exec.Cmd, self string, owner int, key, where, title string) *exec.Cmd {
	if self == "" {
		return cmd
	}
	if key == "" {
		key = noKey // wt.exe drops empty arguments: the rest would shift
	}
	if title == "" {
		title = noKey
	}
	args := append([]string{self, PaneCommand, strconv.Itoa(owner), key, where, title, "--"}, cmd.Args...)
	c := exec.Command(args[0], args[1:]...)
	c.Dir = cmd.Dir
	return c
}

// RunPane is the wrapper's body: start the tool, wait for it, or stop it once the
// owner exits. It returns the exit code for the pane (0 when the owner closed it,
// so the terminal closes the pane instead of showing an error).
func RunPane(args []string) int {
	if len(args) < 6 || args[4] != "--" {
		return 2
	}
	owner, err := strconv.Atoi(args[0])
	if err != nil {
		return 2
	}
	key, where, title := args[1], args[2], args[3]
	if key == noKey {
		key = ""
	}
	// ctrl+c belongs to the tool (Claude clears input / exits on its own). The wrapper
	// catches it so it does not die first and take the pane with it; Notify, not
	// Ignore, so the child still gets the default behaviour.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		for range sig {
		}
	}()
	cmd := exec.Command(args[5], args[6:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if isClaude(args[5]) {
		// the tab shows the chat's name: Claude's own titles are off, the tab hook
		// (_tabcolor) writes the name on every message, so a renamed chat shows too
		k := key
		if k == "" {
			k = noKey
		}
		cmd.Env = append(os.Environ(), "CLAUDE_CODE_DISABLE_TERMINAL_TITLE=1", TabKeyEnv+"="+k)
	}
	if err := cmd.Start(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		return 1
	}
	unmark := mark(key, title, where)
	defer unmark()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var tick <-chan time.Time // nil for a tab: never fires
	if owner > 0 {
		t := time.NewTicker(ownerPoll)
		defer t.Stop()
		tick = t.C
	}
	for {
		select {
		case err := <-done:
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return ee.ExitCode()
			}
			if err != nil {
				return 1
			}
			return 0
		case <-tick:
			if !alive(owner) {
				stopTree(cmd.Process)
				<-done
				return 0
			}
		}
	}
}

// TabKeyEnv tells the tab hook it runs in a tab or pane jedkx-cli opened, and
// which chat key the tab's marker has ("-" for a new chat).
const TabKeyEnv = "JEDKX_TAB_KEY"

func isClaude(bin string) bool {
	b := strings.ToLower(filepath.Base(bin))
	return b == "claude" || b == "claude.exe" || b == "claude.cmd"
}
