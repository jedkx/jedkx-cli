// Package library finds the instruction files that shape your AI tools:
// CLAUDE.md / AGENTS.md, skills, agents, slash commands, output styles and
// prompts — globally and in each project you have chats in. Read-only.
package library

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jedkx/jedkx-cli/internal/paths"
)

// Entry is one file in the library.
type Entry struct {
	Kind  string // instructions, skill, agent, command, style, prompt
	Tool  string // claude, codex
	Scope string // "global" or the project folder's name
	Name  string
	Desc  string
	Path  string
}

type rule struct {
	kind, tool, glob string
	skillDir         bool // name comes from the folder (skills/<name>/SKILL.md)
}

var globalRules = []rule{
	{"instructions", "claude", ".claude/CLAUDE.md", false},
	{"skill", "claude", ".claude/skills/*/SKILL.md", true},
	{"agent", "claude", ".claude/agents/*.md", false},
	{"command", "claude", ".claude/commands/*.md", false},
	{"style", "claude", ".claude/output-styles/*.md", false},
	{"instructions", "codex", ".codex/AGENTS.md", false},
	{"skill", "codex", ".codex/skills/*/SKILL.md", true},
	{"prompt", "codex", ".codex/prompts/*.md", false},
}

var projectRules = []rule{
	{"instructions", "claude", "CLAUDE.md", false},
	{"instructions", "claude", ".claude/CLAUDE.md", false},
	{"skill", "claude", ".claude/skills/*/SKILL.md", true},
	{"agent", "claude", ".claude/agents/*.md", false},
	{"command", "claude", ".claude/commands/*.md", false},
	{"instructions", "codex", "AGENTS.md", false},
}

// Scan lists global files, then those of each project folder (deduplicated).
func Scan(projects []string) []Entry {
	var out []Entry
	seen := map[string]bool{}
	add := func(base, scope string, rules []rule) {
		for _, r := range rules {
			matches, _ := filepath.Glob(filepath.Join(base, filepath.FromSlash(r.glob)))
			for _, p := range matches {
				key := strings.ToLower(filepath.Clean(p))
				if seen[key] {
					continue
				}
				seen[key] = true
				e := Entry{Kind: r.kind, Tool: r.tool, Scope: scope, Path: p}
				e.Name, e.Desc = describe(p)
				if r.skillDir {
					e.Name = filepath.Base(filepath.Dir(p))
				} else if e.Name == "" {
					e.Name = strings.TrimSuffix(filepath.Base(p), ".md")
				}
				out = append(out, e)
			}
		}
	}
	add(paths.Home(), "global", globalRules)
	sort.Strings(projects)
	for _, d := range projects {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			continue
		}
		add(d, filepath.Base(d), projectRules)
	}
	return out
}

// describe reads name/description from YAML front matter, else the first
// heading or line, from the first few KB only.
func describe(p string) (name, desc string) {
	f, err := os.Open(p)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(io16k(f))
	inFront, first := false, true
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if first && l == "---" {
			inFront, first = true, false
			continue
		}
		first = false
		if inFront {
			if l == "---" {
				if desc != "" {
					return name, desc
				}
				inFront = false
				continue
			}
			if k, v, ok := strings.Cut(l, ":"); ok {
				v = strings.Trim(strings.TrimSpace(v), `"'`)
				switch strings.TrimSpace(k) {
				case "name":
					name = v
				case "description":
					desc = v
				}
			}
			continue
		}
		if l != "" && desc == "" {
			desc = strings.TrimSpace(strings.TrimLeft(l, "#>* "))
			return name, desc
		}
	}
	return name, desc
}
