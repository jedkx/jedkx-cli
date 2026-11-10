// Package paths is the single place that decides where jedkx-cli keeps files.
// Install, uninstall and every module go through it, so cleanup never has to guess.
package paths

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/jedkx/jedkx-cli/internal/app"
)

// Home is the user's home directory ("." if it cannot be resolved).
func Home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// ConfigDir holds the user's own data (names, pins, settings).
// Windows: %APPDATA%\jedkx-cli. Removed on uninstall only when the user agrees.
func ConfigDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(Home(), ".config")
	}
	return filepath.Join(d, app.Name)
}

// CacheDir holds rebuildable data (scan index, update state). Always removed on uninstall.
// Windows: %LOCALAPPDATA%\jedkx-cli.
func CacheDir() string {
	d, err := os.UserCacheDir()
	if err != nil {
		d = filepath.Join(Home(), ".cache")
	}
	return filepath.Join(d, app.Name)
}

// InstallDir is where the binary lives after install (per-user, no admin).
// Windows: %LOCALAPPDATA%\Programs\jedkx-cli. Others: ~/.local/share/jedkx-cli.
func InstallDir() string {
	if runtime.GOOS == "windows" {
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "Programs", app.Name)
		}
	}
	return filepath.Join(Home(), ".local", "share", app.Name)
}

// BinDir is the directory placed on PATH. Windows: the install dir itself.
// Others: ~/.local/bin (a symlink-free copy of the binary lives there).
func BinDir() string {
	if runtime.GOOS == "windows" {
		return InstallDir()
	}
	return filepath.Join(Home(), ".local", "bin")
}

// ExeName is the binary file name for this OS.
func ExeName() string {
	if runtime.GOOS == "windows" {
		return app.Name + ".exe"
	}
	return app.Name
}

// InstalledExe is the full path of the installed binary.
func InstalledExe() string { return filepath.Join(BinDir(), ExeName()) }

// Self is the path of the running binary (symlinks resolved).
func Self() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

// SamePath compares two paths the way the OS does (case-insensitive on Windows).
func SamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return equalFold(a, b)
	}
	return a == b
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// Downloads is where exports go: ~/Downloads when it exists, else home.
func Downloads() string {
	d := filepath.Join(Home(), "Downloads")
	if st, err := os.Stat(d); err == nil && st.IsDir() {
		return d
	}
	return Home()
}
