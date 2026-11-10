// Package config stores user settings in ConfigDir/config.json.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jedkx/jedkx-cli/internal/paths"
)

// Config is the full settings file. Zero value is never used directly: Load applies defaults.
type Config struct {
	UpdateCheck bool `json:"update_check"` // on every start, in the background, GitHub Releases only
	AutoUpdate  bool `json:"auto_update"`  // apply updates without asking
	Animation   bool `json:"animation"`    // Nox animation in the TUI
	ShowClaude  bool `json:"show_claude"`  // list Claude Code chats
	ShowCodex   bool `json:"show_codex"`   // list Codex chats
}

// Hidden returns the tools the user chose not to list.
func Hidden(c Config) []string {
	var out []string
	if !c.ShowClaude {
		out = append(out, "claude")
	}
	if !c.ShowCodex {
		out = append(out, "codex")
	}
	return out
}

// Default is what a fresh install uses.
func Default() Config {
	return Config{UpdateCheck: true, AutoUpdate: true, Animation: true, ShowClaude: true, ShowCodex: true}
}

// File is the settings path.
func File() string { return filepath.Join(paths.ConfigDir(), "config.json") }

// Load reads settings; missing file or keys fall back to defaults.
func Load() Config {
	c := Default()
	b, err := os.ReadFile(File())
	if err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

// Save writes settings atomically.
func Save(c Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return WriteAtomic(File(), b)
}

// keys maps "update.check" style names to fields.
var keys = map[string]func(*Config) *bool{
	"update.check": func(c *Config) *bool { return &c.UpdateCheck },
	"update.auto":  func(c *Config) *bool { return &c.AutoUpdate },
	"animation":    func(c *Config) *bool { return &c.Animation },
	"claude":       func(c *Config) *bool { return &c.ShowClaude },
	"codex":        func(c *Config) *bool { return &c.ShowCodex },
}

// Keys lists settable keys.
func Keys() []string {
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Get returns a key's value.
func Get(c Config, key string) (bool, error) {
	f, ok := keys[key]
	if !ok {
		return false, fmt.Errorf("unknown key %q (keys: %s)", key, strings.Join(Keys(), ", "))
	}
	return *f(&c), nil
}

// Set parses on/off/true/false and stores it.
func Set(c *Config, key, value string) error {
	f, ok := keys[key]
	if !ok {
		return fmt.Errorf("unknown key %q (keys: %s)", key, strings.Join(Keys(), ", "))
	}
	switch strings.ToLower(value) {
	case "on", "true", "1", "yes":
		*f(c) = true
	case "off", "false", "0", "no":
		*f(c) = false
	default:
		return fmt.Errorf("value must be on or off, got %q", value)
	}
	return nil
}

// WriteAtomic writes via a temp file + rename so a crash never leaves a half file.
func WriteAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
