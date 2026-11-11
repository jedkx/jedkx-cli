// Package launch starts tool sessions outside the list: beside it (a split pane)
// or in a new tab. Running in the current terminal is done by the UI itself
// (tea.ExecProcess).
//
// Supported hosts, picked from the environment:
//
//	Windows Terminal  WT_SESSION   wt.exe split-pane / new-tab
//	tmux (any OS)     TMUX         tmux split-window / new-window
//
// Anything else has no panes or tabs we can drive; Host() returns HostNone and
// the UI opens sessions in place. A new host (kitty, WezTerm, iTerm2, zellij…)
// is one more case in Host() plus its pane/tab commands below.
package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/setup"
)

// HostKind is the terminal multiplexer that can open panes and tabs for us.
type HostKind int

const (
	HostNone HostKind = iota
	HostWindowsTerminal
	HostTmux
)

// Host detects where jedkx-cli is running. tmux wins: inside tmux, its panes are
// the ones the user sees even when the outer terminal is Windows Terminal.
func Host() HostKind {
	switch {
	case os.Getenv("TMUX") != "":
		return HostTmux
	case os.Getenv("WT_SESSION") != "":
		return HostWindowsTerminal
	}
	return HostNone
}

// CanSplit reports whether SplitPane can work here.
func CanSplit() bool { return Host() != HostNone }

// Ready reports whether the command's binary is on PATH.
func Ready(cmd *exec.Cmd) error {
	if len(cmd.Args) == 0 {
		return errors.New("empty command")
	}
	if _, err := exec.LookPath(cmd.Args[0]); err != nil {
		return errors.New(cmd.Args[0] + " is not on PATH")
	}
	return nil
}

// besideShare is how much of the width the opened session gets; the list keeps the rest.
const besideShare = 75

// SplitPane opens cmd in a pane to the right of the list and returns immediately.
// The pane is owned by this process: it closes when jedkx-cli exits (see pane.go).
// key is the chat's session key ("" for a new chat), used by IsOpen.
func SplitPane(cmd *exec.Cmd, title, key string) error {
	if err := Ready(cmd); err != nil {
		return err
	}
	cmd = wrap(cmd, paths.Self(), os.Getpid(), key, "pane", title) // closes with the list
	switch Host() {
	case HostTmux:
		return tmux(cmd, "split-window", "-h", "-l", fmt.Sprintf("%d%%", besideShare))
	case HostWindowsTerminal:
		return wt(cmd, "split-pane", title, "-V", "-s", fmt.Sprintf("0.%d", besideShare))
	}
	return errors.New("panes need Windows Terminal or tmux")
}

// NewTab opens cmd in a new tab (tmux window) and returns immediately.
func NewTab(cmd *exec.Cmd, title, key string) error {
	if err := Ready(cmd); err != nil {
		return err
	}
	cmd = wrap(cmd, paths.Self(), 0, key, "tab", title) // not owned: stays when the list closes
	switch Host() {
	case HostTmux:
		return tmux(cmd, "new-window", "-n", title)
	case HostWindowsTerminal:
		return wt(cmd, "new-tab", title)
	}
	return errors.New("tabs need Windows Terminal or tmux")
}

func wt(cmd *exec.Cmd, sub, title string, extra ...string) error {
	exe, err := exec.LookPath("wt.exe")
	if err != nil {
		return errors.New("Windows Terminal (wt.exe) not found")
	}
	if err := Ready(cmd); err != nil {
		return err
	}
	// wt.exe splits its command line on ';' — a title like "fix; test" would break it
	title = strings.ReplaceAll(title, ";", `\;`)
	// the chat keeps this title: Claude / Codex would rename the tab otherwise
	args := append([]string{"-w", "0", sub, "--title", title, "--suppressApplicationTitle"}, extra...)
	if setup.ProfileReady() { // same jedkx look in the new tab or pane
		args = append(args, "-p", setup.ProfileName)
	}
	if cmd.Dir != "" {
		args = append(args, "-d", cmd.Dir)
	}
	args = append(args, "--")
	for _, a := range cmd.Args { // ';' would start a new wt command here too
		args = append(args, strings.ReplaceAll(a, ";", `\;`))
	}
	return exec.Command(exe, args...).Start()
}

func tmux(cmd *exec.Cmd, sub string, extra ...string) error {
	if err := Ready(cmd); err != nil {
		return err
	}
	args := append([]string{sub}, extra...)
	if cmd.Dir != "" {
		args = append(args, "-c", cmd.Dir)
	}
	args = append(args, cmd.Args...)
	return exec.Command("tmux", args...).Run()
}

// Focus brings the tab with this title to the front: tmux selects the window,
// Windows Terminal is asked through UI Automation (WT has no command for it).
// false when it could not be found (e.g. you renamed the tab).
func Focus(title string) bool {
	switch Host() {
	case HostTmux:
		return exec.Command("tmux", "select-window", "-t", "="+title).Run() == nil
	case HostWindowsTerminal:
		return focusWT(title)
	}
	return false
}
