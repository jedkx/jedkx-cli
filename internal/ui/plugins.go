package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jedkx/jedkx-cli/internal/plugins"
)

// The plugins & mcp tab: Claude Code's plugins and MCP servers, one per line.
// Changes go through Claude Code's own commands, which take a few seconds, so
// they run in the background and the list is read again afterwards.

type (
	plugListMsg struct {
		items []plugins.Entry
		err   error
	}
	plugDoneMsg struct {
		done string
		err  error
	}
)

func loadPlugins() tea.Cmd {
	return func() tea.Msg {
		items, err := plugins.List()
		return plugListMsg{items, err}
	}
}

func plugAction(done string, fn func() error) tea.Cmd {
	return func() tea.Msg { return plugDoneMsg{done, fn()} }
}

func (m Model) updatePlug(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.statusBad {
		m.status, m.statusBad = "", false
	}
	n := len(m.plugItems)
	if m.plugAsk { // "remove …? y/n"
		m.plugAsk = false
		if k.String() == "y" && m.plugCursor < n {
			e := m.plugItems[m.plugCursor]
			m.plugBusy = "removing " + e.Name + "…"
			return m, plugAction("removed "+e.Name, func() error { return plugins.Remove(e) })
		}
		m.say("kept", false)
		return m, nil
	}
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		return m.setTab(m.tabIndex() + 1)
	case "shift+tab":
		return m.setTab(m.tabIndex() - 1)
	case "esc":
		return m.setTab(m.tool)
	case "?":
		m.help = true
	case "up", "k":
		m.plugCursor = max(0, m.plugCursor-1)
	case "down", "j":
		m.plugCursor = min(n-1, m.plugCursor+1)
	case "g", "home":
		m.plugCursor = 0
	case "end":
		m.plugCursor = n - 1
	case "ctrl+r", "f5":
		m.plugBusy = "asking Claude Code…"
		return m, loadPlugins()
	case "enter", " ", "space", "u", "d", "delete":
		if m.plugBusy != "" || m.plugCursor < 0 || m.plugCursor >= n {
			return m, nil
		}
		e := m.plugItems[m.plugCursor]
		switch k.String() {
		case "u":
			m.plugBusy = "updating " + e.Name + "…"
			return m, plugAction("updated "+e.Name, func() error { return plugins.Update(e) })
		case "d", "delete":
			m.plugAsk = true
			return m, nil
		}
		if e.Kind != "plugin" {
			m.say("MCP servers: d removes one", false)
			return m, nil
		}
		verb := "turned on "
		if e.Enabled {
			verb = "turned off "
		}
		m.plugBusy = "switching " + e.Name + "…"
		return m, plugAction(verb+e.Name, func() error { return plugins.Toggle(e) })
	}
	m.plugCursor = max(0, m.plugCursor)
	if m.plugCursor < m.plugOffset {
		m.plugOffset = m.plugCursor
	}
	if rows := m.libRows(); m.plugCursor >= m.plugOffset+rows {
		m.plugOffset = m.plugCursor - rows + 1
	}
	return m, nil
}

func (m Model) plugView() string {
	var b strings.Builder
	w := m.w
	np, nm := 0, 0
	for _, e := range m.plugItems {
		if e.Kind == "plugin" {
			np++
		} else {
			nm++
		}
	}
	left := " " + sBold.Render("PLUGINS & MCP") + sDim.Render(fmt.Sprintf("  %d plugins · %d mcp", np, nm))
	right := m.tabsFit(w - ansi.StringWidth(left) - 2)
	gap := max(1, w-ansi.StringWidth(left)-ansi.StringWidth(right))
	b.WriteString(ansi.Truncate(left+strings.Repeat(" ", gap)+right, w-1, "…") + "\n" + sRule.Render(strings.Repeat("─", w)) + "\n")
	rows := m.libRows()
	for r := 0; r < rows; r++ {
		i := m.plugOffset + r
		if i >= len(m.plugItems) {
			if i == 0 && r == 0 {
				msg := "nothing here yet"
				if m.plugBusy != "" {
					msg = m.plugBusy
				}
				b.WriteString(ansi.Truncate("  "+sDim.Render(msg), w-1, "…") + "\n")
				continue
			}
			b.WriteString("\n")
			continue
		}
		e := m.plugItems[i]
		bar := "  "
		nmTxt := padRight(ansi.Truncate(e.Name, 39, "…"), 40)
		name := sTitle.Render(nmTxt)
		if i == m.plugCursor {
			bar, name = sSelBar.Render("▌ "), sBold.Render(nmTxt)
		}
		state := sOK.Render("on  ")
		if !e.Enabled {
			state = sDim.Render("off ")
		}
		kind := sNeon.Render(padRight(e.Kind, 8))
		extra := e.Version + " · " + e.Scope
		if e.Kind == "mcp" {
			extra = e.Detail
		}
		line := bar + state + kind + name + sDim.Render(extra)
		b.WriteString(ansi.Truncate(line, w-1, "…") + "\n")
	}
	b.WriteString(sRule.Render(strings.Repeat("─", w)) + "\n")
	switch {
	case m.plugAsk && m.plugCursor < len(m.plugItems):
		b.WriteString(ansi.Truncate("  "+sAccent.Render("remove "+m.plugItems[m.plugCursor].Name+"?")+"  "+keys("y", "remove", "n", "keep"), w-1, "…") + "\n\n")
	case m.plugBusy != "" && len(m.plugItems) > 0:
		b.WriteString(ansi.Truncate("  "+sNeon.Render(m.plugBusy), w-1, "…") + "\n\n")
	default:
		b.WriteString(ansi.Truncate("  "+sDim.Render("changes go through Claude Code; restart open chats to use them"), w-1, "…") + "\n\n")
	}
	l1 := keys("enter", "on / off", "u", "update", "d", "remove")
	l2 := keys("tab", "next tab", "esc", "back to chats", "ctrl+r", "refresh", "?", "help")
	b.WriteString("\n" + ansi.Truncate(" "+l1, w-1, "…") + "\n" + ansi.Truncate(" "+l2, w-1, "…"))
	return b.String()
}
