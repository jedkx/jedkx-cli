package launch

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"time"
)

// Panes opened beside the list belong to it: they run through `jedkx-cli _pane
// <owner pid> -- <tool …>`, a small wrapper that runs the tool in the pane and
// ends it when the list (the owner) is gone. Closing jedkx-cli therefore closes
// the chats it opened beside it; the tool's own exit (ctrl+c, /exit) closes just
// that pane. Tabs are independent and are not wrapped.

// PaneCommand is the internal subcommand name used by the wrapper.
const PaneCommand = "_pane"

// ownerPoll is how often the wrapper checks that the list is still running.
const ownerPoll = time.Second

// owned rewrites cmd so it runs under the wrapper, owned by this process.
func owned(cmd *exec.Cmd, self string) *exec.Cmd {
	if self == "" {
		return cmd
	}
	args := append([]string{self, PaneCommand, strconv.Itoa(os.Getpid()), "--"}, cmd.Args...)
	c := exec.Command(args[0], args[1:]...)
	c.Dir = cmd.Dir
	return c
}

// RunPane is the wrapper's body: start the tool, wait for it, or stop it once the
// owner exits. It returns the exit code for the pane (0 when the owner closed it,
// so the terminal closes the pane instead of showing an error).
func RunPane(args []string) int {
	if len(args) < 3 || args[1] != "--" {
		return 2
	}
	owner, err := strconv.Atoi(args[0])
	if err != nil {
		return 2
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
	cmd := exec.Command(args[2], args[3:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		return 1
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tick := time.NewTicker(ownerPoll)
	defer tick.Stop()
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
		case <-tick.C:
			if !alive(owner) {
				stopTree(cmd.Process)
				<-done
				return 0
			}
		}
	}
}
