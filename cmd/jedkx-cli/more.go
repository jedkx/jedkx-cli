package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/clean"
	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/ui"
)

// ---------------------------------------------------------------- clean

// doClean: what the tools keep on disk, and old chats. Nothing goes without a yes.
func doClean(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	older := fs.Int("older", 0, "delete chats older than this many days (not pinned)")
	yesAll := fs.Bool("yes", false, "do not ask")
	_ = fs.Parse(args)
	r := bufio.NewReader(os.Stdin)
	if *older > 0 {
		return cleanOld(cfg, r, *older, *yesAll)
	}
	spots := clean.Spots()
	if len(spots) == 0 {
		fmt.Println("  nothing to clean")
		return nil
	}
	fmt.Println(ui.Bold("  disk · what the tools keep"))
	fmt.Println()
	var total int64
	for i, s := range spots {
		total += s.Size
		fmt.Printf("   %s  %-9s %-16s %8s  %s\n", ui.Accent(fmt.Sprint(i+1)), s.Tool, s.Name, human(s.Size), ui.Dim(s.What))
	}
	fmt.Printf("\n   %s  %s\n", ui.Dim("total"), human(total))
	fmt.Println(ui.Dim("\n  old chats: jedkx-cli clean --older 30"))
	picks := []int{}
	if *yesAll {
		for i := range spots {
			picks = append(picks, i+1)
		}
	} else {
		picks = askMany(r, "Empty which?", len(spots))
		if len(picks) == 0 {
			fmt.Println("  Nothing changed.")
			return nil
		}
	}
	var freed int64
	for _, p := range picks {
		n, err := clean.Empty(spots[p-1])
		if err != nil {
			fmt.Println("  " + ui.Accent("✗ "+spots[p-1].Name+": "+err.Error()))
		}
		freed += n
	}
	fmt.Println("\n  " + ui.OK("✓ freed "+human(freed)))
	return nil
}

func cleanOld(cfg config.Config, r *bufio.Reader, days int, yesAll bool) error {
	store := meta.Load(meta.File())
	res := catalog.Load(store, config.Hidden(cfg)...)
	cut := time.Now().AddDate(0, 0, -days)
	var old []catalog.Item
	for _, it := range res.Items {
		if !it.Pinned && it.Updated.Before(cut) {
			old = append(old, it)
		}
	}
	if len(old) == 0 {
		fmt.Printf("  no chats older than %d days (pinned ones are kept)\n", days)
		return nil
	}
	var sz int64
	for _, it := range old {
		sz += it.Size
	}
	fmt.Printf("  %d chats older than %d days, %s (pinned ones are kept)\n", len(old), days, human(sz))
	for i, it := range old {
		if i == 8 {
			fmt.Println(ui.Dim(fmt.Sprintf("    … and %d more", len(old)-8)))
			break
		}
		fmt.Println("    " + ui.Dim(it.Updated.Format("2006-01-02")) + "  " + it.Tool + "  " + it.Label())
	}
	if !yesAll {
		fmt.Print("\n  " + ui.Bold("Delete them for good?") + " " + ui.Dim("[y/N]") + " ")
		if !yes(r, false) {
			fmt.Println("  Nothing changed.")
			return nil
		}
	}
	n := 0
	for _, it := range old {
		if a := catalog.Adapter(it.Tool); a != nil && a.Delete(it.Session) == nil {
			n++
		}
	}
	fmt.Println("  " + ui.OK(fmt.Sprintf("✓ deleted %d chats", n)))
	return nil
}

func human(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// ---------------------------------------------------------------- plugins

// doPlugins lists Claude Code plugins and MCP servers and passes changes to
// Claude Code's own commands (`claude plugin …`), never editing its files.
func doPlugins(args []string) error {
	if len(args) >= 2 {
		switch args[0] {
		case "enable", "disable", "uninstall", "update":
			return passThrough("claude", "plugin", args[0], args[1])
		case "remove":
			return passThrough("claude", "plugin", "uninstall", args[1])
		case "mcp-remove":
			return passThrough("claude", "mcp", "remove", args[1])
		}
	}
	out, err := exec.Command("claude", "plugin", "list", "--json").Output()
	if err != nil {
		return errors.New("could not ask Claude Code for its plugins (is claude on PATH?)")
	}
	var list []struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Scope   string `json:"scope"`
		Enabled bool   `json:"enabled"`
	}
	_ = json.Unmarshal(out, &list)
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	fmt.Println(ui.Bold(fmt.Sprintf("  plugins · %d", len(list))))
	for _, p := range list {
		on := ui.Dim("off")
		if p.Enabled {
			on = ui.OK("on ")
		}
		fmt.Printf("   %s  %-48s %s\n", on, p.ID, ui.Dim(p.Version+" · "+p.Scope))
	}
	if mcp := mcpServers(); len(mcp) > 0 {
		fmt.Println("\n" + ui.Bold(fmt.Sprintf("  MCP servers · %d", len(mcp))))
		for _, m := range mcp {
			fmt.Println("   " + m)
		}
	}
	fmt.Println(ui.Dim("\n  jedkx-cli plugins enable|disable|update|remove <id>   ·   jedkx-cli plugins mcp-remove <name>"))
	return nil
}

// mcpServers are the user-level MCP servers in ~/.claude.json (read-only).
func mcpServers() []string {
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
	var out []string
	for name, s := range c.MCP {
		where := s.URL
		if where == "" {
			where = s.Command
		}
		out = append(out, fmt.Sprintf("%-28s %s", name, ui.Dim(strings.TrimSpace(s.Type+" "+where))))
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- login

// doLogin switches the account a tool is signed in with, using its own login.
func doLogin(args []string) error {
	tool := "claude"
	if len(args) > 0 {
		tool = args[0]
	}
	switch tool {
	case "claude":
		return passThrough("claude", "/login")
	case "codex":
		return passThrough("codex", "login")
	}
	return fmt.Errorf("login: claude or codex, not %q", tool)
}

func passThrough(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}
