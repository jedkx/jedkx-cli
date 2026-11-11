package ui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The m menu: things you need now and then, so the list itself keeps few keys.
// It opens as a panel on the right, over the detail side of the list, so the
// chats stay in view; every line has its own key (m then that key).

type menuItem struct {
	key, label string
	act        func(Model) (tea.Model, tea.Cmd)
	section    string // a heading above the first line of a group
}

// press runs a list key as if it was typed.
func press(s string) func(Model) (tea.Model, tea.Cmd) {
	return func(m Model) (tea.Model, tea.Cmd) {
		var k tea.KeyMsg
		switch s {
		case "ctrl+k":
			k = tea.KeyMsg{Type: tea.KeyCtrlK}
		case "ctrl+o":
			k = tea.KeyMsg{Type: tea.KeyCtrlO}
		case "y":
			k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}
		case "ctrl+r":
			k = tea.KeyMsg{Type: tea.KeyCtrlR}
		default:
			k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		}
		return m.updateKeys(k)
	}
}

// self runs jedkx-cli itself in this terminal (the list waits and comes back).
func self(args ...string) func(Model) (tea.Model, tea.Cmd) {
	return func(m Model) (tea.Model, tea.Cmd) {
		exe, err := os.Executable()
		if err != nil {
			m.say(err.Error(), true)
			return m, nil
		}
		return m.run(exec.Command(exe, append(args, "--pause")...), "jedkx-cli", here, "")
	}
}

func (m Model) menuItems() []menuItem {
	archive := "archive the chat"
	if m.archived {
		archive = "restore the chat"
	}
	if len(m.markedKeys()) > 0 {
		archive = "archive / restore the selected"
	}
	pin := "pin to the top"
	if it, ok := m.selected(); ok && it.Pinned {
		pin = "unpin"
	}
	view := "plain list (no project groups)"
	if m.flat {
		view = "group by project"
	}
	return []menuItem{
		{"p", pin, press("p"), "this chat"},
		{"r", "rename", press("r"), ""},
		{"a", archive, press("a"), ""},
		{"#", "tags / a short note", press("#"), ""},
		{"d", "delete (asks first)", press("d"), ""},
		{"o", "open here, full screen", press("o"), ""},
		{"u", "continue in Codex / Claude", press("u"), ""},
		{"ctrl+k", "compact (/compact)", press("ctrl+k"), ""},
		{"e", "export as Markdown", press("e"), ""},
		{"y", "copy the resume command", press("y"), ""},
		{"f", "open the folder", press("f"), ""},
		{".", "open the folder in VS Code", press("ctrl+o"), ""},
		{"s", "sort: recent · name · project · size", press("s"), "list"},
		{"v", view, press("v"), ""},
		{"ctrl+r", "refresh", press("ctrl+r"), ""},
		{"alt+c", "free disk: caches, file history", self("clean"), "tools"},
		{"alt+o", "delete chats older than 30 days", self("clean", "--older", "30"), ""},
		{"alt+l", "sign in with another Claude account", func(m Model) (tea.Model, tea.Cmd) {
			return m.run(exec.Command("claude", "/login"), "claude login", newTab, "")
		}, ""},
		{"!", "notifications", press("!"), ""},
	}
}

func (m Model) updateMenu(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.menuItems()
	switch s := k.String(); s {
	case "esc", "m", "q":
		m.menu = false
	case "ctrl+c":
		return m, tea.Quit
	case "up", "shift+tab":
		m.menuCursor = (m.menuCursor - 1 + len(items)) % len(items)
	case "down", "tab":
		m.menuCursor = (m.menuCursor + 1) % len(items)
	case "enter":
		m.menu = false
		return items[m.menuCursor].act(m)
	default:
		for _, it := range items {
			if it.key == s {
				m.menu = false
				return it.act(m)
			}
		}
	}
	return m, nil
}

// menuWidth is the right panel's width; 0 = too narrow, the menu takes the screen.
func (m Model) menuWidth() int {
	if m.w < 72 {
		return 0
	}
	return min(60, m.w*11/20)
}

// menuLines are the panel's lines, each at most w wide, at most h tall.
func (m Model) menuLines(w, h int) []string {
	fit := func(s string) string { return ansi.Truncate(s, w, "…") }
	sub := "for the chats and the tools"
	if it, ok := m.selected(); ok {
		sub = it.Label()
	}
	head := []string{fit(" " + sBold.Render("MORE") + sDim.Render("  "+sub)), sRule.Render(strings.Repeat("─", w))}
	foot := []string{sRule.Render(strings.Repeat("─", w)), fit(" " + keys("key", "do it (works without m too)", "esc", "close"))}
	items := m.menuItems()
	base := len(head) + len(foot) + len(items)
	headings := h >= base+3 // a heading per group
	gaps := h >= base+5     // and a blank line between groups
	var body []string
	for i, it := range items {
		if it.section != "" && headings {
			if i > 0 && gaps {
				body = append(body, "")
			}
			body = append(body, fit("  "+sDim.Render(strings.ToUpper(it.section))))
		}
		bar, label := "  ", sTitle.Render(it.label)
		if i == m.menuCursor {
			bar, label = sSelBar.Render("▌ "), sBold.Render(it.label)
		}
		body = append(body, fit(bar+sAccent.Render(padRight(it.key, 7))+" "+label))
	}
	// keep the cursor line in view when the panel is short
	room := max(1, h-len(head)-len(foot))
	if len(body) > room {
		at := 0
		for i, l := range body {
			if strings.Contains(l, "▌") {
				at = i
			}
		}
		start := max(0, min(at-room/2, len(body)-room))
		body = body[start : start+room]
	}
	for len(body) < room { // the panel runs down to the footer
		body = append(body, "")
	}
	return append(append(head, body...), foot...)
}

// menuView is the full-screen menu for narrow terminals.
func (m Model) menuView() string {
	return strings.Join(m.menuLines(m.w-1, max(8, m.h-m.headerRows())), "\n")
}

// withMenu lays the menu panel over the right side of the list screen, from
// below the search line down to just above the key hints.
func (m Model) withMenu(screen string) string {
	pw := m.menuWidth()
	lines := strings.Split(screen, "\n")
	top := m.headerRows() + 2
	bottom := len(lines) - 3 // the blank line and two hint lines stay
	if bottom-top < 6 {
		top, bottom = 0, len(lines)
	}
	panel := m.menuLines(pw-2, bottom-top)
	left := m.w - pw
	for i, p := range panel {
		r := top + i
		if r >= bottom {
			break
		}
		base := padRight(ansi.Truncate(lines[r], left, ""), left)
		lines[r] = base + sRule.Render("│") + " " + p
	}
	return strings.Join(lines, "\n")
}
