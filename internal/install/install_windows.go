//go:build windows

package install

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/jedkx/jedkx-cli/internal/app"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + app.Name

func registryKey() string { return `HKCU\` + uninstallKey }

func userPath() (string, uint32, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE)
	if err != nil {
		return "", registry.EXPAND_SZ, err
	}
	defer k.Close()
	v, typ, err := k.GetStringValue("Path")
	if err == registry.ErrNotExist {
		return "", registry.EXPAND_SZ, nil
	}
	return v, typ, err
}

func setUserPath(v string, typ uint32) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if typ == registry.SZ {
		return k.SetStringValue("Path", v)
	}
	return k.SetExpandStringValue("Path", v) // keep %VARS% working
}

func pathHas(dir string) bool {
	v, _, err := userPath()
	return err == nil && hasEntry(v, dir)
}

// addToPath appends dir to the USER path (never the system path). Returns false if already present.
func addToPath(dir string) (bool, error) {
	v, typ, err := userPath()
	if err != nil {
		return false, err
	}
	if hasEntry(v, dir) {
		return false, nil
	}
	if err := setUserPath(addEntry(v, dir), typ); err != nil {
		return false, err
	}
	broadcastEnvChange()
	return true, nil
}

func removeFromPath(dir string) error {
	v, typ, err := userPath()
	if err != nil || !hasEntry(v, dir) {
		return err
	}
	return setUserPath(removeEntry(v, dir), typ)
}

// registerApp adds jedkx-cli to Settings › Apps so it can be removed from there too.
func registerApp(exe string) (string, error) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	size := uint32(0)
	if st, err := os.Stat(exe); err == nil {
		size = uint32(st.Size() / 1024)
	}
	q := `"` + exe + `"`
	for name, val := range map[string]string{
		"DisplayName":          app.Name,
		"DisplayVersion":       app.Version,
		"Publisher":            app.Publisher,
		"DisplayIcon":          exe,
		"InstallLocation":      dirOf(exe),
		"URLInfoAbout":         app.Homepage,
		"UninstallString":      q + " uninstall --yes",
		"QuietUninstallString": q + " uninstall --yes",
	} {
		if err := k.SetStringValue(name, val); err != nil {
			return "", err
		}
	}
	_ = k.SetDWordValue("NoModify", 1)
	_ = k.SetDWordValue("NoRepair", 1)
	_ = k.SetDWordValue("EstimatedSize", size)
	return registryKey(), nil
}

func registryExists() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

func unregisterApp() error {
	err := registry.DeleteKey(registry.CURRENT_USER, uninstallKey)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

// scheduleRemoval deletes dir shortly after this process exits
// (a running .exe cannot delete itself on Windows).
func scheduleRemoval(dir string) error {
	cmd := exec.Command("cmd.exe")
	cmd.Dir = os.TempDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       `cmd.exe /d /c ping 127.0.0.1 -n 3 >nul & rmdir /s /q "` + dir + `"`,
		HideWindow:    true,
		CreationFlags: 0x08000000 | 0x00000008, // CREATE_NO_WINDOW | DETACHED_PROCESS
	}
	return cmd.Start()
}

// broadcastEnvChange tells Explorer and new terminals that PATH changed.
func broadcastEnvChange() {
	env, _ := windows.UTF16PtrFromString("Environment")
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	proc.Call(0xffff, 0x001A, 0, uintptr(unsafe.Pointer(env)), 0x0002, 1000, uintptr(unsafe.Pointer(&result)))
}

// LaunchedStandalone reports a double-click start (the console belongs to us alone).
func LaunchedStandalone() bool {
	ids := make([]uint32, 4)
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")
	n, _, _ := proc.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	return n == 1
}

func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '\\' || p[i] == '/' {
			return p[:i]
		}
	}
	return p
}

// setDisplayVersion updates the version shown in Settings › Apps.
func setDisplayVersion(v string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	_ = k.SetStringValue("DisplayVersion", v)
}
