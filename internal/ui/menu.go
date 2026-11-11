package ui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The m menu: things you need now and then, so the list itself keeps few keys.

type menuItem struct {
	label, key string // key: the list's own shortcut, "" when there is none
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

func menuItems() []menuItem {
	return []menuItem{
		{"continue in the other tool (Claude ↔ Codex)", "u", press("u")},
		{"compact the chat (/compact)", "", press("ctrl+k")},
		{"open the project in VS Code", "", press("ctrl+o")},
		{"tags / a short note", "#", press("#")},
		{"export the chat as Markdown", "e", press("e")},
		{"copy the resume command", "c", press("c")},
		{"free disk: caches, snapshots, file history", "", self("clean")},
		{"delete chats older than 30 days", "", self("clean", "--older", "30")},
		{"token use (days, last 5 hours)", "", self("stats")},
		{"sign in with another Claude account", "", func(m Model) (tea.Model, tea.Cmd) {
			return m.run(exec.Command("claude", "/login"), "claude login", newTab, "")
		}},
		{"notifications", "!", press("!")},
	}
}

func (m Model) updateMenu(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := menuItems()
	switch k.String() {
	case "esc", "m", "q":
		m.menu = false
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.menuCursor = (m.menuCursor - 1 + len(items)) % len(items)
	case "down", "j", "tab":
		m.menuCursor = (m.menuCursor + 1) % len(items)
	case "enter":
		m.menu = false
		return items[m.menuCursor].act(m)
	default: // a digit picks that line
		if s := k.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' && int(s[0]-'1') < len(items) {
			m.menu = false
			return items[s[0]-'1'].act(m)
		}
	}
	return m, nil
}

func (m Model) menuView() string {
	var b strings.Builder
	w := m.w
	sub := "for the selected chat and the tools"
	if it, ok := m.selected(); ok {
		sub = it.Label()
	}
	b.WriteString(ansi.Truncate(" "+sBold.Render("MORE")+sDim.Render("  "+sub), w-1, "…") + "\n" + sRule.Render(strings.Repeat("─", w)) + "\n")
	for i, it := range menuItems() {
		bar, label := "  ", sTitle.Render(it.label)
		if i == m.menuCursor {
			bar, label = sSelBar.Render("▌ "), sBold.Render(it.label)
		}
		num := sDim.Render(string(rune('1'+i)) + "  ")
		if i >= 9 {
			num = "   "
		}
		line := bar + num + label
		if it.key != "" {
			line += sDim.Render("   key " + it.key)
		}
		b.WriteString(ansi.Truncate(line, w-1, "…") + "\n")
	}
	b.WriteString(sRule.Render(strings.Repeat("─", w)) + "\n")
	b.WriteString(ansi.Truncate(" "+keys("enter", "do it", "↑ ↓", "move", "1-9", "pick", "esc", "back"), w-1, "…"))
	return b.String()
}
