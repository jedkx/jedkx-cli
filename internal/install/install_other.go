//go:build !windows

package install

import (
	"os"
	"path/filepath"
	"strings"
)

// On macOS/Linux jedkx-cli does not edit shell profiles: it installs into ~/.local/bin
// (on PATH for most setups) and tells the user if it is not.

func registryKey() string      { return "" }
func registryExists() bool     { return false }
func unregisterApp() error     { return nil }
func broadcastEnvChange()      {}
func LaunchedStandalone() bool { return false }

func registerApp(string) (string, error) { return "", nil }
func setDisplayVersion(string)           {}

func pathHas(dir string) bool {
	for _, e := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(e) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

func addToPath(dir string) (bool, error) {
	if pathHas(dir) {
		return false, nil
	}
	return false, errNotOnPath(dir)
}

func removeFromPath(string) error { return nil }

// A running binary can be deleted on Unix.
func scheduleRemoval(dir string) error { return os.RemoveAll(dir) }

type pathHint string

func (p pathHint) Error() string {
	return "add " + string(p) + " to PATH, e.g. echo 'export PATH=\"" + string(p) + ":$PATH\"' >> ~/." + shell() + "rc"
}

func errNotOnPath(dir string) error { return pathHint(dir) }

func shell() string {
	s := filepath.Base(os.Getenv("SHELL"))
	if strings.Contains(s, "zsh") {
		return "zsh"
	}
	return "bash"
}
