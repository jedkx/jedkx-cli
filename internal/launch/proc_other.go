//go:build !windows

package launch

import (
	"os"
	"syscall"
	"time"
)

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// started stands in for a process start time: 1 while pid runs, 0 when not
// (pids are reused slowly here, so a running pid is taken as the same process).
func started(pid int) int64 {
	if alive(pid) {
		return 1
	}
	return 0
}

// stopTree asks the tool to stop (SIGTERM) and forces it after two seconds.
// RunPane waits for the exit.
func stopTree(p *os.Process) {
	_ = p.Signal(syscall.SIGTERM)
	time.AfterFunc(2*time.Second, func() { _ = p.Kill() })
}

func focusWT(string) bool { return false }
