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
	return []menuItem{
		{"a", archive, press("a")},
		{"u", "continue in the other tool (Claude ↔ Codex)", press("u")},
		{"k", "compact the chat (/compact)", press("ctrl+k")},
		{"v", "open the project in VS Code", press("ctrl+o")},
		{"#", "tags / a short note", press("#")},
		{"e", "export the chat as Markdown", press("e")},
		{"c", "copy the resume command", press("c")},
		{"d", "free disk: caches, file history", self("clean")},
		{"o", "delete chats older than 30 days", self("clean", "--older", "30")},
		{"t", "token use (days, last 5 hours)", self("stats")},
		{"l", "sign in with another Claude account", func(m Model) (tea.Model, tea.Cmd) {
			return m.run(exec.Command("claude", "/login"), "claude login", newTab, "")
		}},
		{"!", "notifications", press("!")},
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
	return min(52, m.w/2)
}

// menuLines are the panel's lines, each at most w wide.
func (m Model) menuLines(w int) []string {
	fit := func(s string) string { return ansi.Truncate(s, w, "…") }
	sub := "for the chats and the tools"
	if it, ok := m.selected(); ok {
		sub = it.Label()
	}
	out := []string{
		fit(" " + sBold.Render("MORE") + sDim.Render("  "+sub)),
		sRule.Render(strings.Repeat("─", w)),
	}
	for i, it := range m.menuItems() {
		bar, label := "  ", sTitle.Render(it.label)
		if i == m.menuCursor {
			bar, label = sSelBar.Render("▌ "), sBold.Render(it.label)
		}
		out = append(out, fit(bar+sAccent.Render(it.key)+"  "+label))
	}
	return append(out,
		sRule.Render(strings.Repeat("─", w)),
		fit(" "+keys("key", "do it", "↑ ↓ enter", "pick", "esc", "close")))
}

// menuView is the full-screen menu for narrow terminals.
func (m Model) menuView() string {
	return strings.Join(m.menuLines(m.w-1), "\n")
}

// withMenu lays the menu panel over the right side of the list screen.
func (m Model) withMenu(screen string) string {
	pw := m.menuWidth()
	lines := strings.Split(screen, "\n")
	panel := m.menuLines(pw - 2)
	top := m.headerRows() + 2 // below Nox, the search line and its rule
	left := m.w - pw
	for i, p := range panel {
		r := top + i
		if r >= len(lines) {
			break
		}
		base := padRight(ansi.Truncate(lines[r], left, ""), left)
		lines[r] = base + sRule.Render("│") + " " + p
	}
	if r := top + len(panel); r < len(lines) {
		base := padRight(ansi.Truncate(lines[r], left, ""), left)
		lines[r] = base + sRule.Render("╰"+strings.Repeat("─", pw-2))
	}
	return strings.Join(lines, "\n")
}
