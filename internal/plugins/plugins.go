// Package plugins lists Claude Code's plugins and MCP servers and changes them
// only through Claude Code's own commands (`claude plugin …`, `claude mcp …`),
// never by editing its files.
package plugins

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jedkx/jedkx-cli/internal/paths"
)

// Entry is one plugin or MCP server.
type Entry struct {
	Kind    string // "plugin" or "mcp"
	Name    string // plugin id (name@marketplace) or server name
	Version string
	Scope   string
	Detail  string // MCP: transport and command / URL
	Enabled bool
}

// List asks Claude Code for its plugins and reads the user's MCP servers.
func List() ([]Entry, error) {
	out, err := exec.Command("claude", "plugin", "list", "--json").Output()
	if err != nil {
		return nil, errors.New("could not ask Claude Code for its plugins (is claude on PATH?)")
	}
	var raw []struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Scope   string `json:"scope"`
		Enabled bool   `json:"enabled"`
	}
	_ = json.Unmarshal(out, &raw)
	var list []Entry
	for _, p := range raw {
		list = append(list, Entry{Kind: "plugin", Name: p.ID, Version: p.Version, Scope: p.Scope, Enabled: p.Enabled})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return append(list, MCP()...), nil
}

// MCP are the user-level MCP servers in ~/.claude.json (read-only).
func MCP() []Entry {
	f := filepath.Join(paths.Home(), ".claude.json")
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		f = filepath.Join(d, ".claude.json")
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return nil
	}
	var c struct {
		MCP map[string]struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			URL     string `json:"url"`
		} `json:"mcpServers"`
	}
	_ = json.Unmarshal(b, &c)
	var out []Entry
	for name, s := range c.MCP {
		where := s.URL
		if where == "" {
			where = s.Command
		}
		out = append(out, Entry{Kind: "mcp", Name: name, Scope: "user", Enabled: true, Detail: strings.TrimSpace(s.Type + " " + where)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Toggle enables a disabled plugin or disables an enabled one.
func Toggle(e Entry) error {
	if e.Kind != "plugin" {
		return errors.New("MCP servers can be removed, not switched off, from here")
	}
	if e.Enabled {
		return claude("plugin", "disable", e.Name)
	}
	return claude("plugin", "enable", e.Name)
}

// Update updates a plugin to its latest version.
func Update(e Entry) error {
	if e.Kind != "plugin" {
		return errors.New("only plugins can be updated")
	}
	return claude("plugin", "update", e.Name)
}

// Remove uninstalls a plugin or removes an MCP server.
func Remove(e Entry) error {
	if e.Kind == "mcp" {
		return claude("mcp", "remove", e.Name, "-s", "user")
	}
	return claude("plugin", "uninstall", e.Name)
}

func claude(args ...string) error {
	out, err := exec.Command("claude", args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = strings.TrimSpace(msg[i+1:])
		}
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}
