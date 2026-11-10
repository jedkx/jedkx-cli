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

// stopTree asks the tool to stop (SIGTERM) and forces it after two seconds.
// RunPane waits for the exit.
func stopTree(p *os.Process) {
	_ = p.Signal(syscall.SIGTERM)
	time.AfterFunc(2*time.Second, func() { _ = p.Kill() })
}
