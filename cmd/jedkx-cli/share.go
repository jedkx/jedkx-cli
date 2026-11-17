package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jedkx/jedkx-cli/internal/install"
	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/share"
	"github.com/jedkx/jedkx-cli/internal/ui"
)

// doShare: jedkx-cli share [status|on|off] [--only skills,md,mcp] [--dry-run]
func doShare(args []string) error {
	verb := "status"
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		verb, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("share", flag.ExitOnError)
	only := fs.String("only", "", "skills, md, mcp (comma separated; default all)")
	dry := fs.Bool("dry-run", false, "show what would change")
	_ = fs.Parse(args)
	parts, err := share.ParseParts(*only)
	if err != nil {
		return err
	}
	switch verb {
	case "status", "":
		printShareStatus()
		return nil
	case "on":
		if *dry {
			for _, l := range share.Plan(parts) {
				fmt.Println("  · " + l)
			}
			fmt.Println(ui.Dim("\n  dry run — nothing was changed"))
			return nil
		}
		if parts.MCP && !install.Installed() {
			// the tools start the server from the installed exe; a downloaded copy may vanish
			return errors.New("install jedkx-cli first (jedkx-cli install) — the apps start the chat server from the installed copy")
		}
		fmt.Print(ui.Banner("share"))
		fmt.Println()
		err := share.Apply(paths.InstalledExe(), parts, os.Stdout)
		fmt.Println("\n  " + ui.Dim("Restart Claude Code and Codex to pick it up · undo: jedkx-cli share off"))
		return err
	case "off":
		if *dry {
			fmt.Println("  · each tool gets its own copy of the shared skills and AGENTS.md back; the MCP server is removed")
			fmt.Println(ui.Dim("\n  dry run — nothing was changed"))
			return nil
		}
		return share.UndoOnly(os.Stdout, parts)
	}
	return fmt.Errorf("share: unknown %q (status, on, off)", verb)
}

func printShareStatus() {
	if !share.Available() {
		fmt.Println("  share needs both Claude Code and Codex on this PC.")
		return
	}
	for _, it := range share.Status() {
		mark := ui.Dim("○")
		if it.On {
			mark = ui.OK("●")
		}
		fmt.Printf("  %s %-22s %s\n", mark, it.Label, ui.Muted(it.Note))
	}
	fmt.Println(ui.Dim("\n  on: jedkx-cli share on [--only skills,md,mcp]   off: jedkx-cli share off   folder: " + paths.ShareDir()))
}
