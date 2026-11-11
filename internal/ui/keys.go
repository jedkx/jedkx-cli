// Input: keys for the list, search, rename and delete; opening sessions.

package ui

import (
	"errors"
	"fmt"
	"github.com/jedkx/jedkx-cli/internal/paths"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/export"
	"github.com/jedkx/jedkx-cli/internal/launch"
	"github.com/jedkx/jedkx-cli/internal/meta"
)

func (m Model) updateSearch(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		if m.search.Value() != "" {
			m.search.SetValue("")
		} else {
			m.searching = false
			m.search.Blur()
		}
		m.refilter()
		return m, nil
	case "enter":
		m.searching = false
		m.search.Blur()
		if _, ok := m.onGroup(); ok {
			m.firstSession()
		}
		return m.open(beside()) // same as enter in the list
	case "up", "down", "pgup", "pgdown", "ctrl+n", "ctrl+p":
		return m.move(k.String()), nil
	case "ctrl+f": // look inside every chat, not only titles and first prompts
		q := strings.TrimSpace(m.search.Value())
		if len([]rune(q)) < 2 {
			m.say("type at least 2 letters, then ctrl+f", true)
			return m, nil
		}
		m.say("searching inside chats…", false)
		items := slices.Clone(m.res.Items) // the list may be re-sorted meanwhile
		return m, func() tea.Msg { return deepMsg{q, deepSearch(items, q)} }
	}
	before := m.search.Value()
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(k)
	if m.search.Value() != before {
		m.refilter()
		if len(m.search.Value())%2 == 0 {
			m.react(moodLookLeft)
		} else {
			m.react(moodLookRight)
		}
	}
	return m, cmd
}

func (m Model) updateRename(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.renaming, m.tagging = false, false
		m.say("cancelled", false)
		return m, nil
	case "enter":
		m.renaming = false
		if m.tagging { // "#": words become tags, found by search
			m.tagging = false
			tags := strings.Fields(m.rename.Value())
			m.edit(func(e *meta.Entry) { e.Tags = tags })
			m.say(fmt.Sprintf("%d tags saved", len(tags)), false)
			return m, nil
		}
		name := strings.TrimSpace(m.rename.Value())
		m.edit(func(e *meta.Entry) { e.Name = name })
		if name == "" {
			m.say("name cleared", false)
		} else {
			m.say("renamed", false)
			m.react(moodHappy)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.rename, cmd = m.rename.Update(k)
	return m, cmd
}

func (m Model) updateKeys(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.statusBad { // an error stays until you press a key
		m.status, m.statusBad = "", false
	}
	if m.showLog {
		switch k.String() {
		case "!", "esc":
			m.showLog = false
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}
	if m.help { // the help screen covers the list: no hidden actions behind it
		switch k.String() {
		case "?", "esc":
			m.help = false
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.help {
			m.help = false
			return m, nil
		}
		if len(m.marked) > 0 {
			m.marked = nil
			m.say("selection cleared", false)
			return m, nil
		}
		if m.search.Value() != "" {
			m.search.SetValue("")
			m.refilter()
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.help = !m.help
	case "!":
		m.showLog = true
	case "/":
		m.searching = true
		return m, m.search.Focus()
	case "tab":
		return m.setTab(m.tabIndex() + 1)
	case "shift+tab":
		return m.setTab(m.tabIndex() - 1)
	case "ctrl+r", "f5": // chats again, and a look for a newer jedkx-cli
		m.loading, m.rescanSays = true, 2
		if m.updState == updInstalled || m.updState == updDownloading {
			return m, m.load()
		}
		return m, tea.Batch(m.load(), checkUpdate(m.cfg))
	case "enter":
		if g, ok := m.onGroup(); ok {
			m.fold(g, !m.folded[g])
			return m, nil
		}
		return m.open(beside())
	case "left", "h":
		m.fold(m.currentGroup(), true)
	case "right", "l":
		m.fold(m.currentGroup(), false)
	case "z":
		m.foldAll()
	case " ", "space":
		m.previewOn = !m.previewOn
		m.previewKey = "" // read again for the current selection
		m.clamp()
	case "e": // the whole chat as Markdown in Downloads
		if keys := m.markedKeys(); len(keys) > 0 {
			var items []catalog.Item
			for _, it := range m.res.Items {
				if slices.Contains(keys, it.Key()) {
					items = append(items, it)
				}
			}
			m.say(fmt.Sprintf("exporting %d chats…", len(items)), false)
			return m, func() tea.Msg {
				var last string
				var errs []error
				for _, it := range items {
					p, err := export.Save(it)
					if err != nil {
						errs = append(errs, err)
					} else {
						last = p
					}
				}
				if last == "" {
					return exportedMsg{"", errors.Join(errs...)}
				}
				return exportedMsg{fmt.Sprintf("%d files in %s", len(items)-len(errs), filepath.Dir(last)), nil}
			}
		}
		if it, ok := m.selected(); ok {
			m.say("exporting…", false)
			return m, func() tea.Msg {
				p, err := export.Save(it)
				return exportedMsg{p, err}
			}
		}
	case "c": // copy the resume command, to run it anywhere
		if it, ok := m.selected(); ok {
			if a := catalog.Adapter(it.Tool); a != nil {
				if err := launch.Copy(launch.CommandLine(a.Resume(it.Session))); err != nil {
					m.say("copy failed: "+err.Error(), true)
				} else {
					m.say("resume command copied", false)
					m.react(moodHappy)
				}
			}
		}
	case "f": // the project folder in the file manager
		if it, ok := m.selected(); ok && it.CWD != "" {
			if err := launch.OpenFolder(it.CWD); err != nil {
				m.say(err.Error(), true)
			} else {
				m.say("folder opened", false)
			}
		}
	case "s":
		m.sortBy = (m.sortBy + 1) % len(sortModes)
		keep := m.selectedKey()
		m.refilter()
		m.selectKey(keep)
		m.say("sorted by "+sortModes[m.sortBy], false)
	case "v":
		m.flat = !m.flat
		keep := m.selectedKey()
		m.refilter()
		m.selectKey(keep)
	case "o":
		return m.open(here)
	case "t":
		return m.open(newTab)
	case "x": // select: this chat, or every chat of the group under the cursor
		m.toggleMark()
	case "ctrl+a": // select every chat in view, or clear when all are
		m.markAll()
	case "d", "delete":
		if keys := m.markedKeys(); len(keys) > 0 {
			m.deleting, m.deleteKeys = true, keys
			m.say(fmt.Sprintf("delete %d selected chats for good?  y / n", len(keys)), true)
			m.react(moodSurprised)
			return m, nil
		}
		if it, ok := m.selected(); ok {
			m.deleting, m.deleteKey = true, it.Key() // y deletes exactly this one
			m.say("delete \""+ansi.Truncate(it.Label(), 40, "…")+"\" for good?  y / n", true)
			m.react(moodSurprised)
		}
	case "n":
		return m.startNew()
	case "p":
		if n := m.bulkEdit(func(e *meta.Entry) { e.Pinned = true }); n > 0 {
			m.say(fmt.Sprintf("pinned %d chats", n), false)
			return m, nil
		}
		if it, ok := m.selected(); ok {
			m.edit(func(e *meta.Entry) { e.Pinned = !e.Pinned })
			if !it.Pinned {
				m.say("pinned", false)
				m.react(moodHappy)
			} else {
				m.say("unpinned", false)
			}
		}
	case "a":
		if n := m.bulkEdit(func(e *meta.Entry) { e.Archived = !m.archived }); n > 0 {
			m.say(fmt.Sprintf("%d chats moved", n), false)
			m.refilter()
			return m, nil
		}
		if it, ok := m.selected(); ok {
			m.edit(func(e *meta.Entry) { e.Archived = !e.Archived })
			if !it.Archived {
				m.say("archived — H shows archived", false)
				m.react(moodSleep)
			} else {
				m.say("restored", false)
			}
		}
	case "#": // tags / a short note, found by search
		if it, ok := m.selected(); ok {
			m.renaming, m.tagging = true, true
			m.rename.SetValue(strings.Join(it.Tags, " "))
			m.rename.CursorEnd()
			return m, m.rename.Focus()
		}
	case "u": // continue in the other tool: Claude ↔ Codex
		return m.handOff()
	case "ctrl+k": // compact: open the chat and run /compact
		if it, ok := m.selected(); ok {
			a := catalog.Adapter(it.Tool)
			if a == nil {
				return m, nil
			}
			cmd := a.Resume(it.Session)
			cmd.Args = append(cmd.Args, "/compact")
			return m.run(cmd, "compact "+it.Label(), newTab, "")
		}
	case "ctrl+o": // the project in VS Code
		if it, ok := m.selected(); ok {
			if err := launch.Editor(it.CWD); err != nil {
				m.say(err.Error(), true)
			} else {
				m.say("opened in the editor", false)
			}
		}
	case "r":
		if it, ok := m.selected(); ok {
			m.renaming = true
			m.rename.SetValue(it.Label())
			m.rename.CursorEnd()
			return m, m.rename.Focus()
		}
	default:
		return m.move(k.String()), nil
	}
	return m, nil
}

func (m Model) move(key string) Model {
	page := max(1, m.listRows()-1)
	switch key {
	case "up", "k", "ctrl+p":
		m.cursor--
	case "down", "j", "ctrl+n":
		m.cursor++
	case "pgup", "ctrl+u":
		m.cursor -= page
	case "pgdown", "ctrl+d":
		m.cursor += page
	case "home", "g":
		m.cursor = 0
	case "end":
		m.cursor = len(m.rows) - 1
	}
	m.clamp()
	return m
}

// where a session opens: this terminal, a new Windows Terminal tab, or a pane beside the list.
type place int

const (
	here place = iota
	newTab
	splitPane
)

func (m Model) open(at place) (tea.Model, tea.Cmd) {
	it, ok := m.selected()
	if !ok {
		return m, nil
	}
	a := catalog.Adapter(it.Tool)
	if a == nil {
		return m, nil
	}
	if o, open := launch.IsOpen(it.Key()); open { // already running: go there, not a second copy
		if o.Where == "pane" {
			m.say("already open beside the list (alt+← → moves between panes)", false)
			return m, nil
		}
		m.say("already open in the tab \""+o.Title+"\"", false)
		title := o.Title
		return m, func() tea.Msg { return focusMsg{title, launch.Focus(title)} }
	}
	return m.run(a.Resume(it.Session), it.Label(), at, it.Key())
}

// handOff continues a chat in the other tool: the chat goes to a Markdown file
// and the other tool starts in the same folder, in a new tab, asked to read it.
// The chat itself is not converted — the tools' files are their own.
func (m Model) handOff() (tea.Model, tea.Cmd) {
	it, ok := m.selected()
	if !ok {
		return m, nil
	}
	other := "codex"
	if it.Tool == "codex" {
		other = "claude"
	}
	a := catalog.Adapter(other)
	if a == nil {
		m.say(other+" is not set up here", true)
		return m, nil
	}
	file, err := export.SaveTo(it, filepath.Join(paths.CacheDir(), "handoff"))
	if err != nil {
		m.say(err.Error(), true)
		return m, nil
	}
	cmd := a.New(it.CWD)
	cmd.Args = append(cmd.Args, "This continues an earlier "+it.Tool+" conversation, saved here: "+file+
		" — read it first, then tell me in two lines where we left off and wait for my next message.")
	return m.run(cmd, other+": "+it.Label(), newTab, "")
}

// startNew opens a fresh session with the selected session's tool, in its directory.
func (m Model) startNew() (tea.Model, tea.Cmd) {
	tool, dir := "claude", ""
	if t := toolFilters[m.tool]; t != "" {
		tool = t
	}
	if it, ok := m.selected(); ok {
		tool, dir = it.Tool, it.CWD
	} else if g, ok := m.onGroup(); ok { // on a project header: start in that project
		for _, i := range m.view {
			if groupOf(m.res.Items[i]) == g {
				dir = m.res.Items[i].CWD
				break
			}
		}
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	a := catalog.Adapter(tool)
	return m.run(a.New(dir), "new "+tool, newTab, "") // a fresh chat gets its own tab
}

// focusMsg: the tab of an already open chat was (or could not be) brought to the front.
type focusMsg struct {
	title string
	ok    bool
}

func (m Model) run(cmd *exec.Cmd, title string, at place, key string) (tea.Model, tea.Cmd) {
	if err := launch.Ready(cmd); err != nil {
		m.say(err.Error(), true)
		m.react(moodSurprised)
		return m, nil
	}
	if at != here {
		open, done := launch.NewTab, "opened in a new tab"
		if at == splitPane {
			open, done = launch.SplitPane, "opened beside the list"
		}
		if err := open(cmd, ansi.Truncate(title, 30, "…"), key); err != nil {
			m.say(err.Error(), true)
			return m, nil
		}
		m.say(done, false)
		m.react(moodHappy)
		return m, nil
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return execDoneMsg{err} })
}

func (m *Model) edit(fn func(*meta.Entry)) {
	idx, ok := m.selectedIndex()
	if !ok {
		return
	}
	it := &m.res.Items[idx]
	if err := m.store.Update(it.Key(), fn); err != nil {
		m.say("save failed: "+err.Error(), true)
		return
	}
	it.Entry = m.store.Get(it.Key())
	key := it.Key()
	catalog.Sort(m.res.Items)
	m.refilter()
	m.selectKey(key)
}

func beside() place {
	if launch.CanSplit() {
		return splitPane
	}
	return here
}

func (m Model) confirmDelete(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.deleting = false
	if keys := m.deleteKeys; len(keys) > 0 {
		m.deleteKeys = nil
		if k.String() != "y" {
			m.say("kept", false)
			return m, nil
		}
		done, failed := 0, 0
		for _, it := range m.res.Items {
			if !slices.Contains(keys, it.Key()) {
				continue
			}
			if a := catalog.Adapter(it.Tool); a != nil && a.Delete(it.Session) == nil {
				done++
			} else {
				failed++
			}
		}
		m.marked = nil
		msg := fmt.Sprintf("deleted %d chats", done)
		if failed > 0 {
			msg += fmt.Sprintf(", %d could not be deleted", failed)
		}
		m.say(msg, failed > 0)
		m.react(moodSleep)
		return m, m.load()
	}
	it, ok := m.selected()
	if ok && it.Key() != m.deleteKey { // the list changed under the question
		ok = false
	}
	if k.String() != "y" || !ok {
		m.say("kept", false)
		return m, nil
	}
	a := catalog.Adapter(it.Tool)
	if a == nil {
		return m, nil
	}
	if err := a.Delete(it.Session); err != nil {
		m.say(err.Error(), true)
		return m, nil
	}
	m.say("deleted", false)
	m.react(moodSleep)
	return m, m.load()
}
