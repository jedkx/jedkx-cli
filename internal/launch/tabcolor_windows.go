//go:build windows

package launch

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32      = windows.NewLazySystemDLL("kernel32.dll")
	freeConsole   = kernel32.NewProc("FreeConsole")
	attachConsole = kernel32.NewProc("AttachConsole")
)

// writeToClaudeConsole writes s to the console of the claude.exe that runs this
// hook: hooks get pipes, not the terminal, so the sequence must go to Claude's
// console directly.
func writeToClaudeConsole(s string) {
	pid := ancestor("claude.exe")
	if pid == 0 {
		return
	}
	freeConsole.Call()
	if r, _, _ := attachConsole.Call(uintptr(pid)); r == 0 {
		return
	}
	name, _ := windows.UTF16PtrFromString("CONOUT$")
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	b := []byte(s)
	var n uint32
	_ = windows.WriteFile(h, b, &n, nil)
}

// ancestor is the pid of the nearest parent process named exe (0 = none).
func ancestor(exe string) uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)
	parent, names := map[uint32]uint32{}, map[uint32]string{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		parent[e.ProcessID] = e.ParentProcessID
		names[e.ProcessID] = windows.UTF16ToString(e.ExeFile[:])
	}
	p := uint32(windows.GetCurrentProcessId())
	for i := 0; i < 12; i++ {
		q, ok := parent[p]
		if !ok {
			return 0
		}
		p = q
		if strings.EqualFold(names[p], exe) {
			return p
		}
	}
	return 0
}
