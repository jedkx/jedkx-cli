// jedkx-cli — a light terminal home for your AI coding sessions (Claude Code, Codex).
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jedkx/jedkx-cli/internal/app"
	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/install"
	"github.com/jedkx/jedkx-cli/internal/launch"
	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/setup"
	"github.com/jedkx/jedkx-cli/internal/source"
	"github.com/jedkx/jedkx-cli/internal/stats"
	"github.com/jedkx/jedkx-cli/internal/statusline"
	"github.com/jedkx/jedkx-cli/internal/ui"
	"github.com/jedkx/jedkx-cli/internal/update"
)

const usage = `jedkx-cli — your AI coding sessions, one keystroke away.

Usage:
  jedkx-cli                   open the session manager
  jedkx-cli .                 open the list with only this folder's chats
  jedkx-cli last              continue the latest chat started in this folder
  jedkx-cli stats [--days 7]  token use per day (Claude Code)
  jedkx-cli sessions [query]  print sessions (--json, --tool claude|codex, --all)
  jedkx-cli update [--check]  install the latest release (or only check)
  jedkx-cli config [key [on|off]]  e.g. config codex off  (hide Codex chats)
  jedkx-cli install           install / repair this copy (per-user, no admin; --setup [--only claude|terminal], --no-setup)
  jedkx-cli setup             jedkx look for Claude Code + Windows Terminal (--only claude|terminal, --undo)
  jedkx-cli uninstall         remove everything jedkx-cli added (--dry-run, --purge, --yes)
  jedkx-cli statusline        Claude Code status line (settings.json statusLine)
  jedkx-cli nox               say hi to Nox
  jedkx-cli version
`

func main() {
	// Internal: the wrapper that runs a chat in a pane owned by the list.
	if len(os.Args) > 1 && os.Args[1] == launch.PaneCommand {
		os.Exit(launch.RunPane(os.Args[2:]))
	}
	// Claude calls this on every message: no config, no update work, never fail.
	if len(os.Args) > 1 && os.Args[1] == "statusline" {
		fmt.Print(statusline.Render(os.Stdin))
		return
	}
	update.Cleanup()
	setup.RefreshTerminal() // a new version may bring a changed terminal profile
	setup.RefreshClaude()   // … or a changed Claude Code theme
	install.SyncVersion()
	cfg := config.Load()

	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	var err error
	switch cmd {
	case "":
		if install.LaunchedStandalone() && !install.RunningInstalled() {
			err = wizard()
			break
		}
		err = ui.Run(cfg)
	case ".":
		dir, _ := os.Getwd()
		err = ui.RunQuery(cfg, dir) // only this folder's chats
	case "last":
		err = resumeLast(cfg)
	case "stats":
		err = doStats(cfg, args)
	case "sessions", "ls":
		err = sessions(args)
	case "update":
		err = doUpdate(args)
	case "config":
		err = doConfig(cfg, args)
	case "install":
		err = doInstall(args)
	case "setup":
		err = doSetup(args)
	case "uninstall":
		err = doUninstall(args)
	case "nox":
		err = ui.RunNox()
	case "version", "--version", "-v":
		fmt.Println(app.Name, app.Version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "jedkx-cli:", err)
		os.Exit(1)
	}
}

// wizard runs when jedkx-cli.exe is double-clicked. One menu: everything, Claude
// only, Codex only, terminal only or just the app — it sets both what is listed
// and which settings are touched. Touched settings are backed up next
// to this file first. The window never closes on its own: errors wait for Enter.
func wizard() (err error) {
	r := bufio.NewReader(os.Stdin)
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("unexpected error: %v", p)
			fail(r, err)
		}
	}()
	backupDir := filepath.Dir(paths.Self())

	fmt.Print(ui.Banner("setup"))
	fmt.Println()
	if have := install.InstalledVersion(); have != "" && !install.RunningInstalled() {
		// already on this PC: the likely wish is to update it, settings untouched
		if update.Newer(app.Version, have) {
			fmt.Println("  jedkx-cli " + ui.Bold(have) + " is installed; this file is " + ui.Accent(app.Version) + ".")
			fmt.Print("\n  " + ui.Bold("Update it?") + " " + ui.Dim("your settings, names and pins stay  [Y/n]") + " ")
			if yes(r, true) {
				fmt.Println()
				if err := install.Install(os.Stdout); err != nil {
					return fail(r, fmt.Errorf("update failed: %w", err))
				}
				fmt.Println("\n  " + ui.OK("✓ updated to "+app.Version+".") + " Open a new terminal and type " + ui.Accent("jedkx-cli"))
				if setup.TerminalApplied() && runtime.GOOS == "windows" {
					fmt.Println()
					terminalHint()
				}
				pause(r)
				return nil
			}
		} else {
			fmt.Println("  jedkx-cli " + ui.Bold(have) + " is already installed " + ui.Dim("(this file: "+app.Version+")") + ".")
			if tag, err := update.Check(3 * time.Second); err == nil && update.Newer(tag, have) {
				// a newer release is out than both: offer it
				fmt.Print("\n  " + ui.Bold(tag+" is out. Update the installed copy?") + " " + ui.Dim("[Y/n]") + " ")
				if yes(r, true) {
					fmt.Println("  " + ui.Accent("updating to "+tag+"…"))
					got, err := update.ApplyTo(paths.InstalledExe(), have, func(s string) { fmt.Println(ui.Dim("  · " + s)) })
					if err != nil {
						return fail(r, fmt.Errorf("update failed: %w", err))
					}
					fmt.Println("\n  " + ui.OK("✓ updated to "+got+".") + " Open a new terminal and type " + ui.Accent("jedkx-cli"))
					pause(r)
					return nil
				}
			}
		}
		fmt.Println()
	}
	fmt.Println("  Installs for your user only (no admin) into " + ui.Dim(paths.InstallDir()) + " and adds it to PATH.")
	fmt.Println("  Pick one or more (e.g. 2 4) — Enter takes the first:")
	fmt.Println("  " + ui.Rule(66))
	type choice struct {
		what, detail  string
		parts         setup.Parts
		claude, codex bool // whose chats the list shows
	}
	choices := []choice{
		{"everything", "Claude Code + Codex chats, Claude Code look, terminal look", setup.All, true, true},
		{"Claude only", "Claude Code chats + Claude Code look; no Codex, terminal untouched", setup.Parts{Claude: true}, true, false},
		{"Codex only", "Codex chats; Claude Code and the terminal untouched", setup.Parts{}, false, true},
		{"terminal only", "all chats + a jedkx-cli terminal profile, grey tabs; Claude Code untouched", setup.Parts{Terminal: true}, true, true},
		{"just the app", "all chats; no other setting changes", setup.Parts{}, true, true},
	}
	for i, c := range choices {
		fmt.Println("   " + ui.Accent(fmt.Sprint(i+1)) + "  " + ui.Bold(fmt.Sprintf("%-14s", c.what)) + ui.Muted(c.detail))
	}
	last := len(choices)
	installed := install.Installed()
	if installed { // already on this machine: offer to take it off again
		last++
		fmt.Println("  " + ui.Rule(66))
		fmt.Println("   " + ui.Accent(fmt.Sprint(last)) + "  " + ui.Bold(fmt.Sprintf("%-14s", "uninstall")) + ui.Muted("remove jedkx-cli and put back every setting it changed"))
	}
	fmt.Println("  " + ui.Rule(66))
	fmt.Println("  " + ui.Dim("touched settings are backed up next to this file first · undo: jedkx-cli setup --undo"))
	picks := askMany(r, "Choose", last)
	pick := 0
	if len(picks) > 0 {
		pick = picks[0]
	}
	if pick == 0 {
		fmt.Println("  Nothing changed.")
		pause(r)
		return nil
	}
	if installed && len(picks) == 1 && pick == last {
		fmt.Print("\n  " + ui.Bold("Remove jedkx-cli from this PC?") + " " + ui.Dim("[y/N]") + " ")
		if !yes(r, false) {
			fmt.Println("  Nothing changed.")
			pause(r)
			return nil
		}
		fmt.Println()
		// asks before deleting your own names / pins; Claude Code and Windows Terminal are put back
		if err := install.Uninstall(install.Options{}, r, os.Stdout); err != nil {
			return fail(r, fmt.Errorf("uninstall failed: %w", err))
		}
		pause(r)
		return nil
	}
	// several picks add up: "2 4" = Claude Code look + terminal profile. Only
	// "Claude only" / "Codex only" limit whose chats are listed.
	var parts setup.Parts
	show := map[string]bool{}
	limited := false
	for _, p := range picks {
		if installed && p == last {
			continue // uninstall only on its own
		}
		c := choices[p-1]
		parts.Claude = parts.Claude || c.parts.Claude
		parts.Terminal = parts.Terminal || c.parts.Terminal
		if c.claude != c.codex {
			limited = true
			show["claude"] = show["claude"] || c.claude
			show["codex"] = show["codex"] || c.codex
		}
	}
	if !limited {
		show = map[string]bool{"claude": true, "codex": true}
	}
	if len(picks) > 1 {
		var names []string
		for _, p := range picks {
			if !(installed && p == last) {
				names = append(names, choices[p-1].what)
			}
		}
		fmt.Println("  " + ui.Dim("→ "+strings.Join(names, " + ")))
	}

	total := 1 // install
	if parts.Claude || parts.Terminal {
		total = 3 // backup, install, restyle
	}
	n := 0
	step := func(what string) {
		n++
		fmt.Println("\n  " + ui.Accent(fmt.Sprintf("▌ %d/%d", n, total)) + "  " + ui.Bold(what))
	}
	backup := ""
	if parts.Claude || parts.Terminal {
		step("backup")
		backup, err = setup.Backup(backupDir, parts)
		if err != nil { // the exe's folder may be read-only: try Downloads
			backup, err = setup.Backup(paths.Downloads(), parts)
		}
		if err != nil {
			return fail(r, fmt.Errorf("backup failed, nothing was changed: %w", err))
		}
		if backup != "" {
			fmt.Println("  ✓ saved       " + backup)
		} else {
			fmt.Println("  " + ui.Dim("· nothing to back up yet"))
		}
	}
	step("install")
	if err := install.Install(os.Stdout); err != nil {
		return fail(r, fmt.Errorf("install failed: %w", err))
	}
	cfg := config.Load()
	cfg.ShowClaude, cfg.ShowCodex = show["claude"], show["codex"]
	if err := config.Save(cfg); err != nil {
		fmt.Println("  ! settings    " + err.Error())
	} else if !cfg.ShowClaude || !cfg.ShowCodex {
		fmt.Println("  ✓ list        only " + strings.Join(shown(cfg), " + ") + " chats")
	}
	if parts.Claude || parts.Terminal {
		step("restyle")
		if err := setup.Apply(paths.InstalledExe(), parts, os.Stdout); err != nil {
			fmt.Println("\n  " + ui.Accent("Some of the jedkx look could not be applied (see ! above).") + " jedkx-cli itself is installed.")
			fmt.Println("  " + ui.Dim("Fix it and run: jedkx-cli setup"))
		}
	}

	if backup != "" {
		fmt.Print("\n  " + ui.Bold("Delete the backup?") + " " + ui.Dim("setup --undo works without it  [y/N]") + " ")
		if yes(r, false) {
			if err := os.RemoveAll(backup); err != nil {
				fmt.Println("  " + ui.Accent("could not delete it: "+err.Error()))
			} else {
				fmt.Println("  backup deleted")
			}
		} else {
			fmt.Println("  kept: " + backup)
		}
	}
	fmt.Println("\n  " + ui.Rule(58))
	if parts.Terminal && runtime.GOOS == "windows" {
		terminalHint()
	}
	fmt.Println("  " + ui.OK("✓ ready.") + " Close every Windows Terminal and Claude Code window,")
	fmt.Println("  open Windows Terminal again and type " + ui.Accent("jedkx-cli") + ".")
	fmt.Println("  " + ui.Dim("You can delete the downloaded file."))
	pause(r)
	return nil
}

// yes reads a y/n answer; Enter gives def.
func yes(r *bufio.Reader, def bool) bool {
	ans, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "":
		return def
	case "y", "yes", "e", "evet":
		return true
	}
	return false
}

// fail prints the error plainly and waits, so a double-clicked window stays open.
func fail(r *bufio.Reader, err error) error {
	fmt.Println("\n  " + ui.Accent("✗ "+err.Error()))
	pause(r)
	return err
}

func pause(r *bufio.Reader) {
	fmt.Print("\n  press Enter to close ")
	_, _ = r.ReadString('\n')
}

func sessions(args []string) error {
	fs := flag.NewFlagSet("sessions", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "JSON output")
	tool := fs.String("tool", "", "claude or codex")
	all := fs.Bool("all", false, "include archived")
	_ = fs.Parse(args)
	q := strings.Join(fs.Args(), " ")

	res := catalog.Load(meta.Load(meta.File()), config.Hidden(config.Load())...)
	var out []catalog.Item
	for _, it := range res.Items {
		if (it.Archived && !*all) || (*tool != "" && it.Tool != *tool) || !catalog.Matches(it, q) {
			continue
		}
		out = append(out, it)
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, it := range out {
		pin := " "
		if it.Pinned {
			pin = "●"
		}
		fmt.Printf("%s %-6s %s  %-16.16s %-36s %s\n", pin, it.Tool, it.Updated.Format("2006-01-02 15:04"), it.Project(), it.ID, it.Label())
	}
	return nil
}

func doUpdate(args []string) error {
	if app.Version == "dev" {
		return errors.New("development build — updates come from releases")
	}
	if len(args) > 0 && args[0] == "--check" {
		v, err := update.Check(10 * time.Second)
		if err != nil {
			return err
		}
		if update.Newer(v, app.Version) {
			fmt.Println(v, "available → jedkx-cli update")
		} else {
			fmt.Println("up to date:", app.Version)
		}
		return nil
	}
	v, err := update.Apply(func(s string) { fmt.Println(ui.Dim("  · " + s)) })
	if errors.Is(err, update.ErrUpToDate) {
		fmt.Println("up to date:", app.Version)
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Println(ui.OK("✓ updated to "+v), ui.Dim("(old version is removed on next start)"))
	return nil
}

func doConfig(cfg config.Config, args []string) error {
	switch len(args) {
	case 0:
		for _, k := range config.Keys() {
			v, _ := config.Get(cfg, k)
			fmt.Printf("%-14s %s\n", k, onOff(v))
		}
		fmt.Println(ui.Dim("file: " + config.File()))
		return nil
	case 1:
		v, err := config.Get(cfg, args[0])
		if err == nil {
			fmt.Println(onOff(v))
		}
		return err
	default:
		if err := config.Set(&cfg, args[0], args[1]); err != nil {
			return err
		}
		return config.Save(cfg)
	}
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func doUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	var o install.Options
	fs.BoolVar(&o.DryRun, "dry-run", false, "show what would be removed")
	fs.BoolVar(&o.Purge, "purge", false, "also delete your data without asking")
	fs.BoolVar(&o.Yes, "yes", false, "do not ask (keeps your data unless --purge)")
	_ = fs.Parse(args)
	return install.Uninstall(o, os.Stdin, os.Stdout)
}

func doInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	yes := fs.Bool("setup", false, "also apply the jedkx look to Claude Code + Windows Terminal, without asking")
	no := fs.Bool("no-setup", false, "do not touch Claude Code or Windows Terminal")
	only := fs.String("only", "", "with --setup: claude or terminal")
	_ = fs.Parse(args)
	parts, err := parseOnly(*only)
	if err != nil {
		return err
	}
	fmt.Print(ui.Banner("installing"))
	if err := install.Install(os.Stdout); err != nil {
		return err
	}
	switch {
	case *yes:
		fmt.Println()
		if err := setup.Apply(paths.InstalledExe(), parts, os.Stdout); err != nil {
			return fmt.Errorf("installed, but the jedkx look failed: %w", err)
		}
	case !*no:
		offer(bufio.NewReader(os.Stdin), parts, true)
	}
	fmt.Println("\n  " + ui.OK("✓ done.") + " open a new terminal and type " + ui.Accent("jedkx-cli"))
	return nil
}

// offer explains the optional setup and applies it on yes. Without a terminal
// (piped, scripted) it only prints how to run it later.
func offer(r *bufio.Reader, parts setup.Parts, cliHint bool) {
	fmt.Println()
	fmt.Println("  Optional — the jedkx look:")
	if parts.Claude {
		fmt.Println("    · Claude Code: jedkx status line and theme (your current ones are saved)")
	}
	if parts.Terminal && runtime.GOOS == "windows" {
		fmt.Println("    · Windows Terminal: jedkx colour scheme + a \"jedkx-cli\" profile")
	}
	fmt.Println("  " + ui.Dim("Undo any time: jedkx-cli setup --undo (uninstall does it too)."))
	if !interactive() {
		fmt.Println("  Run it with: " + ui.Accent("jedkx-cli setup"))
		return
	}
	fmt.Print("\n  Apply now? [Y/n] ")
	ans, _ := r.ReadString('\n')
	ans = strings.ToLower(strings.TrimSpace(ans))
	if ans != "" && ans != "y" && ans != "yes" && ans != "e" && ans != "evet" {
		if cliHint {
			fmt.Println("  Skipped. Later: " + ui.Accent("jedkx-cli setup"))
		}
		return
	}
	_ = setup.Apply(paths.InstalledExe(), parts, os.Stdout) // Apply prints what failed
}

// parseOnly turns --only claude|terminal into the parts to apply ("" = all).
func parseOnly(s string) (setup.Parts, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "all":
		return setup.All, nil
	case "claude":
		return setup.Parts{Claude: true}, nil
	case "terminal", "wt":
		return setup.Parts{Terminal: true}, nil
	}
	return setup.Parts{}, fmt.Errorf("--only must be claude or terminal, got %q", s)
}

func interactive() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func doSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	undo := fs.Bool("undo", false, "put Claude Code and Windows Terminal back as they were")
	only := fs.String("only", "", "claude or terminal (default: both)")
	_ = fs.Parse(args)
	parts, err := parseOnly(*only)
	if err != nil {
		return err
	}
	if *undo {
		return setup.UndoOnly(os.Stdout, parts)
	}
	if !install.Installed() {
		// the status line and the terminal profile point at the installed exe;
		// pointing them at a downloaded file breaks both once it is deleted
		return errors.New("install jedkx-cli first: jedkx-cli install")
	}
	exe := paths.InstalledExe()
	fmt.Print(ui.Banner("setup"))
	fmt.Println()
	if err := setup.Apply(exe, parts, os.Stdout); err != nil {
		return err
	}
	fmt.Println("\n  " + ui.OK("✓ done.") + " Restart Claude Code to see the status line and theme.")
	if parts.Terminal && runtime.GOOS == "windows" {
		fmt.Println()
		terminalHint()
	}
	return nil
}

// terminalHint: jedkx-cli adds its profile but leaves your default alone, so
// say loudly how to switch.
func terminalHint() {
	fmt.Println("  " + ui.Alert("!! The jedkx terminal look is NOT your default yet."))
	fmt.Println("  " + ui.Alert("!! "+setup.TerminalHint))
	fmt.Println("  " + ui.Dim("   (or open it once from the ⌄ menu next to the tabs)"))
	fmt.Println()
}

// ask reads a number 1..n; Enter = 1, q / n / no = 0 (cancel). Asks again on a typo.
// askMany reads one or more choices ("2", "2 4", "2,4", "2-4" = 2,3,4).
// nil means cancel.
func askMany(r *bufio.Reader, prompt string, n int) []int {
	for {
		fmt.Print("\n  " + ui.Bold(prompt) + " " + ui.Dim(fmt.Sprintf("[1-%d, several: 2 4, enter = 1, q = cancel]", n)) + " ")
		line, err := r.ReadString('\n')
		a := strings.ToLower(strings.TrimSpace(line))
		switch {
		case a == "":
			if err != nil {
				return nil
			}
			return []int{1}
		case a == "q" || a == "n" || a == "no" || a == "h" || a == "hayır":
			return nil
		}
		if v, ok := parsePicks(a, n); ok {
			return v
		}
		fmt.Println("  " + ui.Accent(fmt.Sprintf("type numbers from 1 to %d, e.g. 2 or 2 4", n)))
	}
}

// parsePicks turns "2 4", "2,4" or "2-4" into sorted, distinct numbers in 1..n.
func parsePicks(a string, n int) ([]int, bool) {
	seen := map[int]bool{}
	for _, f := range strings.FieldsFunc(a, func(r rune) bool { return r == ' ' || r == ',' || r == ';' || r == '+' }) {
		lo, hi := f, f
		if i := strings.Index(f, "-"); i > 0 {
			lo, hi = f[:i], f[i+1:]
		}
		x, e1 := strconv.Atoi(lo)
		y, e2 := strconv.Atoi(hi)
		if e1 != nil || e2 != nil || x < 1 || y > n || x > y {
			return nil, false
		}
		for v := x; v <= y; v++ {
			seen[v] = true
		}
	}
	if len(seen) == 0 {
		return nil, false
	}
	var out []int
	for v := 1; v <= n; v++ {
		if seen[v] {
			out = append(out, v)
		}
	}
	return out, true
}

func ask(r *bufio.Reader, prompt string, n int) int {
	for {
		fmt.Print("\n  " + ui.Bold(prompt) + " " + ui.Dim(fmt.Sprintf("[1-%d, enter = 1, q = cancel]", n)) + " ")
		line, err := r.ReadString('\n')
		a := strings.ToLower(strings.TrimSpace(line))
		switch {
		case a == "":
			if err != nil { // no input at all (closed stdin): do not loop
				return 0
			}
			return 1
		case a == "q" || a == "n" || a == "no" || a == "h" || a == "hayır":
			return 0
		}
		var v int
		if _, e := fmt.Sscan(a, &v); e == nil && v >= 1 && v <= n {
			return v
		}
		fmt.Println("  " + ui.Accent(fmt.Sprintf("type a number from 1 to %d", n)))
	}
}

// shown lists the tools whose chats the list shows.
func shown(c config.Config) []string {
	var out []string
	if c.ShowClaude {
		out = append(out, "Claude Code")
	}
	if c.ShowCodex {
		out = append(out, "Codex")
	}
	return out
}

// resumeLast continues the most recent chat whose folder is the current one.
func resumeLast(cfg config.Config) error {
	dir, _ := os.Getwd()
	key := catalog.Item{Session: source.Session{CWD: dir}}.ProjectKey()
	res := catalog.Load(meta.Load(meta.File()), config.Hidden(cfg)...)
	var last *catalog.Item
	for i, it := range res.Items {
		if it.ProjectKey() == key && (last == nil || it.Updated.After(last.Updated)) {
			last = &res.Items[i]
		}
	}
	if last == nil {
		return errors.New("no chat in this folder yet — start one with claude or codex")
	}
	a := catalog.Adapter(last.Tool)
	if a == nil {
		return errors.New("unknown tool " + last.Tool)
	}
	fmt.Println(ui.Dim("  continuing: ") + last.Label())
	cmd := a.Resume(last.Session)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func doStats(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	days := fs.Int("days", 7, "how many days back")
	_ = fs.Parse(args)
	if *days < 1 {
		*days = 1
	}
	res := catalog.Load(meta.Load(meta.File()), config.Hidden(cfg)...)
	fmt.Println(ui.Bold(fmt.Sprintf("  token use · last %d days", *days)))
	fmt.Println()
	stats.Print(os.Stdout, stats.Collect(res.Items, *days))
	return nil
}
