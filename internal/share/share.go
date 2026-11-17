// Package share lets Claude Code and Codex use the same skills, the same
// global instructions and see each other's chats (`jedkx-cli share`).
//
// One copy lives in paths.ShareDir(); both tools point at it, so a change made
// in either app shows up in the other at once:
//
//	~/.claude/skills, ~/.codex/skills  → junction / symlink to share/skills
//	~/.codex/AGENTS.md                 → symlink (or hard link) to share/AGENTS.md
//	~/.claude/CLAUDE.md                → one marked "@…/share/AGENTS.md" import
//	MCP server "jedkx-cli"             → added with the tools' own `mcp add`
//
// Like setup it is opt-in, remembers what it did in share.json and Undo puts
// everything back: each tool gets real folders / files again holding the
// shared content, so nothing is lost. The share folder itself is user data and
// is only removed by uninstall when the user agrees.
package share

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jedkx/jedkx-cli/internal/app"
	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/paths"
)

// ServerName is the MCP server name registered in both tools.
const ServerName = app.Name

// MCPCommand is the hidden subcommand that serves MCP over stdio.
const MCPCommand = "_mcp"

const (
	blockStart = "<!-- jedkx-cli share: start (managed by jedkx-cli, `jedkx-cli share off` removes it) -->"
	blockEnd   = "<!-- jedkx-cli share: end -->"
)

func claudeDir() string { return filepath.Join(paths.Home(), ".claude") }
func codexDir() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	return filepath.Join(paths.Home(), ".codex")
}

// SkillsDir and AgentsFile are the shared copies.
func SkillsDir() string  { return filepath.Join(paths.ShareDir(), "skills") }
func AgentsFile() string { return filepath.Join(paths.ShareDir(), "AGENTS.md") }

func backupRoot() string { return filepath.Join(paths.ShareDir(), "backup") }
func stateFile() string  { return filepath.Join(paths.ConfigDir(), "share.json") }

func toolSkills() map[string]string {
	return map[string]string{
		"claude": filepath.Join(claudeDir(), "skills"),
		"codex":  filepath.Join(codexDir(), "skills"),
	}
}
func codexAgents() string { return filepath.Join(codexDir(), "AGENTS.md") }
func claudeMD() string    { return filepath.Join(claudeDir(), "CLAUDE.md") }

// Parts are the pieces the user can switch on one by one.
type Parts struct{ Skills, Instructions, MCP bool }

// All is every part.
var All = Parts{Skills: true, Instructions: true, MCP: true}

// Any reports whether at least one part is picked.
func (p Parts) Any() bool { return p.Skills || p.Instructions || p.MCP }

// ParseParts turns "skills,md,mcp" into Parts ("" or "all" = All).
func ParseParts(s string) (Parts, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "all" {
		return All, nil
	}
	var p Parts
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '+' }) {
		switch f {
		case "skills", "skill":
			p.Skills = true
		case "md", "instructions", "agents", "claude.md", "agents.md":
			p.Instructions = true
		case "mcp", "chats", "sessions":
			p.MCP = true
		default:
			return Parts{}, fmt.Errorf("unknown part %q (use skills, md, mcp)", f)
		}
	}
	return p, nil
}

type state struct {
	Skills       bool            `json:"skills"`
	Instructions bool            `json:"instructions"`
	AgentsLink   string          `json:"agents_link,omitempty"`   // symlink | hardlink
	ClaudeMDNew  bool            `json:"claude_md_new,omitempty"` // CLAUDE.md did not exist before
	MCP          map[string]bool `json:"mcp,omitempty"`           // tool → registered by us
	Since        time.Time       `json:"since,omitempty"`
}

func loadState() state {
	var s state
	if b, err := os.ReadFile(stateFile()); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func saveState(s state) error {
	if !s.Skills && !s.Instructions && len(s.MCP) == 0 {
		err := os.Remove(stateFile())
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return config.WriteAtomic(stateFile(), b)
}

// Available reports whether both tools are on this machine (share needs both).
func Available() bool {
	return dirExists(claudeDir()) && dirExists(codexDir())
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// Applied reports whether anything is shared right now (for uninstall).
func Applied() bool {
	s := loadState()
	return s.Skills || s.Instructions || len(s.MCP) > 0
}

// Item is one line of Status.
type Item struct {
	Part  string // skills, md, mcp
	Label string
	On    bool
	Note  string // what is off, or how to repair it
}

// Status says, per part, whether both tools use the shared copy.
func Status() []Item {
	st := loadState()
	var out []Item

	sk := Item{Part: "skills", Label: "skills"}
	var off []string
	for _, tool := range []string{"claude", "codex"} {
		if !pointsAt(toolSkills()[tool], SkillsDir()) {
			off = append(off, tool)
		}
	}
	sk.On = len(off) == 0
	if !sk.On {
		if st.Skills {
			sk.Note = "not linked for " + strings.Join(off, ", ") + " — turn it on again to repair"
		} else {
			sk.Note = "each tool has its own skills folder"
		}
	} else {
		sk.Note = fmt.Sprintf("%d shared", countSkills())
	}
	out = append(out, sk)

	md := Item{Part: "md", Label: "AGENTS.md / CLAUDE.md"}
	codexOK := pointsAt(codexAgents(), AgentsFile())
	claudeOK := hasBlock(claudeMD())
	md.On = codexOK && claudeOK
	switch {
	case md.On:
		md.Note = "one file: " + AgentsFile()
	case st.Instructions && !codexOK:
		md.Note = "Codex AGENTS.md was replaced (an editor saved a new file) — turn it on again to merge it back"
	case st.Instructions:
		md.Note = "CLAUDE.md lost its import line — turn it on again to repair"
	default:
		md.Note = "each tool has its own instructions"
	}
	out = append(out, md)

	reg := mcpRegistered()
	mc := Item{Part: "mcp", Label: "chats (MCP)", On: reg["claude"] && reg["codex"]}
	switch {
	case mc.On:
		mc.Note = "both apps can list, read and search each other's chats"
	case reg["claude"] || reg["codex"]:
		var miss []string
		for _, t := range []string{"claude", "codex"} {
			if !reg[t] {
				miss = append(miss, t)
			}
		}
		mc.Note = "not added in " + strings.Join(miss, ", ")
	default:
		mc.Note = "the apps cannot see each other's chats"
	}
	out = append(out, mc)
	return out
}

func countSkills() int {
	n := 0
	ents, _ := os.ReadDir(SkillsDir())
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			n++
		}
	}
	return n
}

// Plan lists what Apply would do, without changing anything.
func Plan(p Parts) []string {
	var out []string
	if p.Skills {
		for _, tool := range []string{"claude", "codex"} {
			dir := toolSkills()[tool]
			switch {
			case pointsAt(dir, SkillsDir()):
				out = append(out, "skills  "+tool+": already shared")
			case dirExists(dir):
				names := entries(dir)
				out = append(out, fmt.Sprintf("skills  %s: move %d item(s) from %s into %s, then link it", tool, len(names), dir, SkillsDir()))
			default:
				out = append(out, "skills  "+tool+": link "+dir+" → "+SkillsDir())
			}
		}
		if c := conflicts(); len(c) > 0 {
			out = append(out, "skills  ! same name, different content (rename one first): "+strings.Join(c, ", "))
		}
	}
	if p.Instructions {
		out = append(out, "md      "+codexAgents()+" → "+AgentsFile())
		out = append(out, "md      "+claudeMD()+": one import line for "+AgentsFile())
	}
	if p.MCP {
		reg := mcpRegistered()
		for _, tool := range []string{"claude", "codex"} {
			if reg[tool] {
				out = append(out, "mcp     "+tool+": already added")
			} else {
				out = append(out, "mcp     "+tool+": "+tool+" mcp add "+ServerName)
			}
		}
	}
	return out
}

func entries(dir string) []string {
	var out []string
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		out = append(out, e.Name())
	}
	return out
}

// conflicts are skill names that exist in more than one place with different content.
func conflicts() []string {
	seen := map[string]string{} // name → first folder holding it
	if dirExists(SkillsDir()) {
		for _, n := range entries(SkillsDir()) {
			seen[n] = filepath.Join(SkillsDir(), n)
		}
	}
	var out []string
	for _, tool := range []string{"claude", "codex"} {
		dir := toolSkills()[tool]
		if !dirExists(dir) || pointsAt(dir, SkillsDir()) {
			continue
		}
		for _, n := range entries(dir) {
			p := filepath.Join(dir, n)
			if first, ok := seen[n]; ok {
				if !sameEntry(first, p) {
					out = append(out, n)
				}
				continue
			}
			seen[n] = p
		}
	}
	sort.Strings(out)
	return out
}

func sameEntry(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	if err1 != nil || err2 != nil || sa.IsDir() != sb.IsDir() {
		return false
	}
	if sa.IsDir() {
		return sameTree(a, b)
	}
	x, _ := os.ReadFile(a)
	y, _ := os.ReadFile(b)
	return bytes.Equal(x, y)
}

// Apply switches on the picked parts; exe is the jedkx-cli binary the tools
// start as their MCP server. Safe to run again: it repairs what drifted.
func Apply(exe string, p Parts, out io.Writer) error {
	if !Available() {
		return errors.New("share needs both Claude Code (~/.claude) and Codex (~/.codex) on this PC")
	}
	st := loadState()
	if st.Since.IsZero() {
		st.Since = time.Now()
	}
	var errs []error
	if p.Skills {
		err := applySkills()
		report(out, "skills", fmt.Sprintf("%d shared  (%s)", countSkills(), SkillsDir()), err)
		if err == nil {
			st.Skills = true
		}
		errs = append(errs, err)
	}
	if p.Instructions {
		err := applyInstructions(&st)
		report(out, "md", "Codex AGENTS.md + Claude CLAUDE.md use "+AgentsFile(), err)
		if err == nil {
			st.Instructions = true
		}
		errs = append(errs, err)
	}
	if p.MCP {
		errs = append(errs, applyMCP(exe, &st, out))
	}
	errs = append(errs, saveState(st))
	return errors.Join(errs...)
}

func report(out io.Writer, what, detail string, err error) {
	if err != nil {
		fmt.Fprintf(out, "  ! %-8s %v\n", what, err)
		return
	}
	fmt.Fprintf(out, "  ✓ %-8s %s\n", what, detail)
}

func stamp() string { return time.Now().Format("20060102-150405") }

func applySkills() error {
	if c := conflicts(); len(c) > 0 {
		return fmt.Errorf("same skill name, different content: %s — rename or delete one, nothing was changed", strings.Join(c, ", "))
	}
	if err := os.MkdirAll(SkillsDir(), 0o755); err != nil {
		return err
	}
	bk := filepath.Join(backupRoot(), stamp())
	for _, tool := range []string{"claude", "codex"} {
		dir := toolSkills()[tool]
		if pointsAt(dir, SkillsDir()) {
			continue
		}
		if isLink(dir) { // a link somewhere else: not ours to change
			return fmt.Errorf("%s is already a link to another folder; remove it first", dir)
		}
		if dirExists(dir) {
			if err := copyTree(dir, filepath.Join(bk, tool+"-skills")); err != nil {
				return fmt.Errorf("backup of %s: %w", dir, err)
			}
			for _, n := range entries(dir) {
				src, dst := filepath.Join(dir, n), filepath.Join(SkillsDir(), n)
				if exists(dst) { // same content (conflicts() checked): drop the duplicate
					if err := os.RemoveAll(src); err != nil {
						return err
					}
					continue
				}
				if err := os.Rename(src, dst); err != nil {
					return fmt.Errorf("move %s: %w", src, err)
				}
			}
			if err := os.Remove(dir); err != nil {
				return fmt.Errorf("%s is not empty after the move: %w", dir, err)
			}
		} else if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return err
		}
		if err := linkDir(dir, SkillsDir()); err != nil {
			return fmt.Errorf("link %s: %w", dir, err)
		}
	}
	return nil
}

func applyInstructions(st *state) error {
	shared := AgentsFile()
	codex, claude := codexAgents(), claudeMD()
	_, claudeRest := splitBlock(readString(claude))
	if sh := strings.TrimSpace(readString(shared)); sh != "" {
		// after an undo CLAUDE.md starts with the shared text: do not keep it twice
		claudeRest = strings.TrimPrefix(strings.TrimSpace(claudeRest), sh)
	}
	bk := filepath.Join(backupRoot(), stamp())

	if !exists(shared) {
		content := ""
		if c := readString(codex); !isLink(codex) {
			content = c
		}
		if rest := strings.TrimSpace(claudeRest); rest != "" && rest != strings.TrimSpace(content) {
			if content != "" {
				content = strings.TrimRight(content, "\n") + "\n\n"
			}
			content += rest + "\n"
			claudeRest = "" // it lives in the shared file now
		}
		if err := writeInPlace(shared, content); err != nil {
			return err
		}
	} else if exists(codex) && !pointsAt(codex, shared) {
		// drift: an editor replaced the linked file. Keep the newer text.
		a, _ := os.Stat(codex)
		b, _ := os.Stat(shared)
		if a != nil && b != nil && a.ModTime().After(b.ModTime()) {
			if err := writeInPlace(shared, readString(codex)); err != nil {
				return err
			}
		}
	}

	if !pointsAt(codex, shared) {
		if exists(codex) {
			if err := copyFile(codex, filepath.Join(bk, "codex-AGENTS.md")); err != nil && !isLink(codex) {
				return fmt.Errorf("backup of %s: %w", codex, err)
			}
			if err := os.Remove(codex); err != nil {
				return err
			}
		}
		kind, err := linkFile(codex, shared)
		if err != nil {
			return fmt.Errorf("link %s: %w", codex, err)
		}
		st.AgentsLink = kind
	}

	if !hasBlock(claude) {
		if exists(claude) {
			if err := copyFile(claude, filepath.Join(bk, "claude-CLAUDE.md")); err != nil {
				return fmt.Errorf("backup of %s: %w", claude, err)
			}
		} else {
			st.ClaudeMDNew = true
		}
		body := block() + "\n"
		if r := strings.TrimSpace(claudeRest); r != "" {
			body += "\n" + r + "\n"
		}
		if err := writeInPlace(claude, body); err != nil {
			return err
		}
	}
	return nil
}

func block() string {
	return blockStart + "\n@" + filepath.ToSlash(AgentsFile()) + "\n" + blockEnd
}

func hasBlock(p string) bool {
	s := readString(p)
	return strings.Contains(s, blockStart) && strings.Contains(s, "@"+filepath.ToSlash(AgentsFile()))
}

// splitBlock returns our block (if any) and the rest of the file.
func splitBlock(s string) (ours, rest string) {
	i := strings.Index(s, blockStart)
	if i < 0 {
		return "", s
	}
	j := strings.Index(s[i:], blockEnd)
	if j < 0 {
		return "", s
	}
	j += i + len(blockEnd)
	return s[i:j], strings.TrimLeft(s[:i]+s[j:], "\r\n")
}

func readString(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

// writeInPlace rewrites p without replacing the file, so hard links stay intact.
func writeInPlace(p, s string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(s), 0o644)
}

// mcpRegistered reads (never writes) where each tool lists its MCP servers.
func mcpRegistered() map[string]bool {
	reg := map[string]bool{}
	f := filepath.Join(paths.Home(), ".claude.json")
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		f = filepath.Join(d, ".claude.json")
	}
	var c struct {
		MCP map[string]json.RawMessage `json:"mcpServers"`
	}
	if b, err := os.ReadFile(f); err == nil && json.Unmarshal(b, &c) == nil {
		_, reg["claude"] = c.MCP[ServerName]
	}
	toml := readString(filepath.Join(codexDir(), "config.toml"))
	reg["codex"] = strings.Contains(toml, "[mcp_servers."+ServerName+"]") ||
		strings.Contains(toml, `[mcp_servers."`+ServerName+`"]`)
	return reg
}

// mcpArgs are the tools' own commands; jedkx-cli never edits their config files.
func mcpAddArgs(tool, exe string) []string {
	if tool == "claude" {
		return []string{"mcp", "add", "--scope", "user", ServerName, "--", exe, MCPCommand}
	}
	return []string{"mcp", "add", ServerName, "--", exe, MCPCommand}
}

func mcpRemoveArgs(tool string) []string {
	if tool == "claude" {
		return []string{"mcp", "remove", "--scope", "user", ServerName}
	}
	return []string{"mcp", "remove", ServerName}
}

// lookTool finds the tool's CLI; tests replace it.
var lookTool = exec.LookPath

var runTool = func(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

// ManualMCP is what to add by hand when a tool's CLI is not on PATH.
func ManualMCP(tool, exe string) string {
	return tool + " " + strings.Join(quoteAll(mcpAddArgs(tool, exe)), " ")
}

func quoteAll(a []string) []string {
	out := make([]string, len(a))
	for i, s := range a {
		if strings.ContainsAny(s, " \t") {
			s = `"` + s + `"`
		}
		out[i] = s
	}
	return out
}

func applyMCP(exe string, st *state, out io.Writer) error {
	if st.MCP == nil {
		st.MCP = map[string]bool{}
	}
	reg := mcpRegistered()
	var errs []error
	for _, tool := range []string{"claude", "codex"} {
		label := "mcp " + tool
		if reg[tool] {
			report(out, label, "already added", nil)
			continue
		}
		bin, err := lookTool(tool)
		if err != nil {
			fmt.Fprintf(out, "  · %-8s %s CLI not on PATH — add it once by hand:  %s\n", label, tool, ManualMCP(tool, exe))
			continue
		}
		err = runTool(bin, mcpAddArgs(tool, exe)...)
		report(out, label, "server \""+ServerName+"\" added", err)
		if err == nil {
			st.MCP[tool] = true
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Undo turns every part off. Safe to run when nothing is shared.
func Undo(out io.Writer) error { return UndoOnly(out, All) }

// UndoOnly turns the picked parts off; each tool gets real files again.
func UndoOnly(out io.Writer, p Parts) error {
	st := loadState()
	var errs []error
	if p.Skills {
		err := undoSkills()
		if err == nil {
			st.Skills = false
		}
		reportOff(out, "skills", "each tool has its own copy again", err)
		errs = append(errs, err)
	}
	if p.Instructions {
		err := undoInstructions(st)
		if err == nil {
			st.Instructions, st.AgentsLink, st.ClaudeMDNew = false, "", false
		}
		reportOff(out, "md", "AGENTS.md and CLAUDE.md are separate files again", err)
		errs = append(errs, err)
	}
	if p.MCP {
		for tool, ours := range st.MCP {
			if !ours {
				continue
			}
			bin, err := lookTool(tool)
			if err == nil {
				err = runTool(bin, mcpRemoveArgs(tool)...)
			} else {
				err = fmt.Errorf("%s CLI not on PATH — remove it by hand: %s %s", tool, tool, strings.Join(mcpRemoveArgs(tool), " "))
			}
			reportOff(out, "mcp "+tool, "server removed", err)
			if err == nil {
				delete(st.MCP, tool)
			}
			errs = append(errs, err)
		}
	}
	errs = append(errs, saveState(st))
	return errors.Join(errs...)
}

func reportOff(out io.Writer, what, detail string, err error) {
	if err != nil {
		fmt.Fprintf(out, "  ! %-8s %v\n", what, err)
		return
	}
	fmt.Fprintf(out, "  ✓ %-8s %s\n", what, detail)
}

func undoSkills() error {
	for _, tool := range []string{"claude", "codex"} {
		dir := toolSkills()[tool]
		if !pointsAt(dir, SkillsDir()) {
			continue
		}
		if err := unlink(dir); err != nil {
			return err
		}
		if err := copyTree(SkillsDir(), dir); err != nil {
			return fmt.Errorf("copy skills back into %s: %w", dir, err)
		}
	}
	return nil
}

func undoInstructions(st state) error {
	shared := AgentsFile()
	content := readString(shared)
	codex, claude := codexAgents(), claudeMD()
	if pointsAt(codex, shared) {
		if err := os.Remove(codex); err != nil {
			return err
		}
		if err := writeInPlace(codex, content); err != nil {
			return err
		}
	}
	if hasBlock(claude) {
		_, rest := splitBlock(readString(claude))
		body := strings.TrimRight(content, "\r\n")
		if r := strings.TrimSpace(rest); r != "" {
			if body != "" {
				body += "\n\n"
			}
			body += r
		}
		if strings.TrimSpace(body) == "" && st.ClaudeMDNew {
			return os.Remove(claude)
		}
		if err := writeInPlace(claude, body+"\n"); err != nil {
			return err
		}
	}
	return nil
}
