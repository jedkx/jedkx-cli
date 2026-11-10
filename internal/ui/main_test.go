package ui

import (
	"os"
	"testing"
)

// TestMain points every config and cache directory at a temp folder, so no
// test can write the user's own names, pins or archive (meta.File()).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "jedkx-ui-test")
	if err != nil {
		panic(err)
	}
	for _, k := range []string{"APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "HOME", "USERPROFILE"} {
		_ = os.Setenv(k, dir)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
