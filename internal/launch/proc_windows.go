//go:build windows

package launch

import (
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/windows"
)

const stillActive = 259 // STILL_ACTIVE

func alive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil {
		return false
	}
	return code == stillActive
}

// stopTree ends the tool and anything it started (node, shells, MCP servers).
func stopTree(p *os.Process) {
	if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.Pid)).Run(); err != nil {
		_ = p.Kill()
	}
}
