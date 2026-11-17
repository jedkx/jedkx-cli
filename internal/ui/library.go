package ui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jedkx/jedkx-cli/internal/install"
	"github.com/jedkx/jedkx-cli/internal/launch"
	"github.com/jedkx/jedkx-cli/internal/library"
	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/share"
)

// The library screen: every skill / instruction file the tools read, one per line.

// libShown is what the library tab lists: every skill and md file.
func (m Model) libShown() []library.Entry { return m.libItems }

func (m Model) updateLib(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.statusBad {
		m.status, m.statusBad = "", false
	}
	shown := m.libShown()
	n := len(shown)
	if m.shareAsk {
		m.shareAsk = false
		if k.String() != "y" && k.String() != "e" {
			m.say("nothing changed", false)
			return m, nil
		}
		m.shareBusy = true
		on := !shareOn(m.shareItems)
		if on {
			m.say("sharing skills, AGENTS.md and chats…", false)
		} else {
			m.say("giving each tool its own copy back…", false)
		}
		return m, toggleShare(on)
	}
	switch k.String() {
	case "S":
		switch {
		case m.shareBusy:
		case m.shareItems == nil:
			m.say("share needs both Claude Code and Codex on this PC", true)
		case !install.Installed():
			m.say("install jedkx-cli first: the apps start its chat server from the installed copy", true)
		default:
			m.shareAsk = true
		}
		return m, nil
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		return m.setTab(m.tabIndex() + 1)
	case "shift+tab":
		return m.setTab(m.tabIndex() - 1)
	case "esc":
		return m.setTab(m.tool) // back to the chats tab you came from
	case "?":
		m.help = true
	case "up", "k":
		m.libCursor = max(0, m.libCursor-1)
	case "down", "j":
		m.libCursor = min(n-1, m.libCursor+1)
	case "pgup":
		m.libCursor = max(0, m.libCursor-m.libRows())
	case "pgdown":
		m.libCursor = min(n-1, m.libCursor+m.libRows())
	case "g", "home":
		m.libCursor = 0
	case "end":
		m.libCursor = n - 1
	case "enter", "f", "c":
		if m.libCursor < 0 || m.libCursor >= n {
			return m, nil
		}
		e := shown[m.libCursor]
		var err error
		switch k.String() {
		case "enter":
			err = launch.OpenFolder(e.Path) // opens the file in its default app
			m.say("opened "+filepath.Base(e.Path), false)
		case "f":
			err = launch.OpenFolder(filepath.Dir(e.Path))
			m.say("folder opened", false)
		case "c":
			err = launch.Copy(e.Path)
			m.say("path copied", false)
		}
		if err != nil {
			m.say(err.Error(), true)
		}
	case "ctrl+r", "f5":
		return m, m.loadLibrary()
	}
	m.libCursor = max(0, m.libCursor)
	if m.libCursor < m.libOffset {
		m.libOffset = m.libCursor
	}
	if rows := m.libRows(); m.libCursor >= m.libOffset+rows {
		m.libOffset = m.libCursor - rows + 1
	}
	return m, nil
}

func (m Model) libRows() int { return max(1, m.h-m.headerRows()-2-3-3-1) }

// shareDoneMsg: share on / off finished.
type shareDoneMsg struct {
	done string
	err  error
}

// loadShare reads what is shared (a few small files); nil when share cannot work here.
func loadShare() []share.Item {
	if !share.Available() {
		return nil
	}
	return share.Status()
}

// shareOn: every part is shared (then S turns it off, else on / repair).
func shareOn(items []share.Item) bool {
	for _, it := range items {
		if !it.On {
			return false
		}
	}
	return len(items) > 0
}

func toggleShare(on bool) tea.Cmd {
	return func() tea.Msg {
		if on {
			err := share.Apply(paths.InstalledExe(), share.All, io.Discard)
			return shareDoneMsg{"shared · restart Claude Code and Codex to pick it up", err}
		}
		return shareDoneMsg{"not shared any more · each tool has its own copy", share.Undo(io.Discard)}
	}
}

// shareLine is the one line above the list: what is shared, and the key.
func (m Model) shareLine(w int) string {
	if m.shareItems == nil {
		return ansi.Truncate(sDim.Render("  share  needs both Claude Code and Codex"), w-1, "…")
	}
	line := "  " + sBold.Render("share") + " "
	for _, it := range m.shareItems {
		dot := sDim.Render("○ ")
		if it.On {
			dot = sNeon.Render("● ")
		}
		line += " " + dot + sMuted.Render(it.Part)
	}
	switch {
	case m.shareAsk && shareOn(m.shareItems):
		line += "   " + sAccent.Render("stop sharing? each tool keeps a copy  y/n")
	case m.shareAsk:
		line += "   " + sAccent.Render("share skills, AGENTS.md and chats between both apps? y/n")
	case m.shareBusy:
		line += "   " + sDim.Render("working…")
	case shareOn(m.shareItems):
		line += "   " + sDim.Render("S stop sharing")
	default:
		line += "   " + sDim.Render("S share")
		for _, it := range m.shareItems {
			if !it.On && w >= 100 {
				line += sDim.Render(" · " + it.Note)
				break
			}
		}
	}
	return ansi.Truncate(line, w-1, "…")
}

func (m Model) libView() string {
	var b strings.Builder
	w := m.w
	shown := m.libShown()
	left := " " + sBold.Render("LIBRARY") + sDim.Render(fmt.Sprintf("  %d", len(shown)))
	if w >= 110 {
		left += sDim.Render(" · skills, agents, commands, CLAUDE.md / AGENTS.md")
	}
	right := m.tabsFit(w - ansi.StringWidth(left) - 2)
	gap := max(1, w-ansi.StringWidth(left)-ansi.StringWidth(right))
	title := ansi.Truncate(left+strings.Repeat(" ", gap)+right, w, "")
	b.WriteString(ansi.Truncate(title, w-1, "…") + "\n" + sRule.Render(strings.Repeat("─", w)) + "\n")
	b.WriteString(m.shareLine(w) + "\n")
	rows := m.libRows()
	for r := 0; r < rows; r++ {
		i := m.libOffset + r
		if i >= len(shown) {
			if i == 0 && r == 0 {
				b.WriteString(ansi.Truncate("  "+sDim.Render("nothing here yet"), w-1, "…") + "\n")
				continue
			}
			b.WriteString("\n")
			continue
		}
		e := shown[i]
		bar := "  "
		nm := padRight(ansi.Truncate(e.Name, 21, "…"), 22) // aligned columns: kind · name · where · what
		name := sTitle.Render(nm)
		if i == m.libCursor {
			bar, name = sSelBar.Render("▌ "), sBold.Render(nm)
		}
		kind := sNeon.Render(padRight(e.Kind, 13))
		if e.Kind == "instructions" {
			kind = sAccent.Render(padRight("instructions", 13))
		}
		scope := sDim.Render(padRight(ansi.Truncate(e.Tool+" · "+e.Scope, 23, "…"), 24))
		line := bar + kind + name + scope
		if w >= 90 && e.Desc != "" {
			line += sMuted.Render(e.Desc)
		}
		b.WriteString(ansi.Truncate(line, w-1, "…") + "\n")
	}
	// detail: description and path of the selected file
	b.WriteString(sRule.Render(strings.Repeat("─", w)) + "\n")
	if m.libCursor < len(shown) {
		e := shown[m.libCursor]
		b.WriteString(ansi.Truncate("  "+sDim.Render("› ")+sMuted.Render(e.Desc), w-1, "…") + "\n")
		b.WriteString(ansi.Truncate("  "+sDim.Render(e.Path), w-1, "…") + "\n")
	} else {
		b.WriteString("\n\n")
	}
	l1 := keys("enter", "open file", "f", "folder", "c", "copy path", "S", "share")
	l2 := keys("tab", "next tab", "esc", "back to chats", "ctrl+r", "rescan", "?", "help")
	b.WriteString("\n" + ansi.Truncate(" "+l1, w-1, "…") + "\n" + ansi.Truncate(" "+l2, w-1, "…"))
	return b.String()
}
