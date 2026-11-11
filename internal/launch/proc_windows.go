//go:build windows

package launch

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

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

// focusWT selects the Windows Terminal tab named title via UI Automation. It
// runs only when you open a chat that is already open, so the PowerShell start
// (~0.5 s) is paid then and never otherwise.
func focusWT(title string) bool {
	const script = `Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes
$A=[System.Windows.Automation.AutomationElement]; $P=[System.Windows.Automation.PropertyCondition]
$tab=New-Object System.Windows.Automation.AndCondition((New-Object $P($A::ControlTypeProperty,[System.Windows.Automation.ControlType]::TabItem)),(New-Object $P($A::NameProperty,$env:JEDKX_TAB)))
foreach($w in $A::RootElement.FindAll('Children',(New-Object $P($A::ClassNameProperty,'CASCADIA_HOSTING_WINDOW_CLASS')))){
  $t=$w.FindFirst('Descendants',$tab)
  if($t){ $t.GetCurrentPattern([System.Windows.Automation.SelectionItemPattern]::Pattern).Select(); try{$w.SetFocus()}catch{}; exit 0 }
}
exit 1`
	c := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	c.Env = append(os.Environ(), "JEDKX_TAB="+title)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return c.Run() == nil
}
