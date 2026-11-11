// Rendering: header with Nox, search bar, list, detail, footer hints, help.

package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/jedkx/jedkx-cli/internal/app"
	"github.com/jedkx/jedkx-cli/internal/launch"
)

// ---------------------------------------------------------------- view

func (m Model) View() string {
	if m.w == 0 {
		return ""
	}
	if m.help {
		return m.helpView()
	}
	var b strings.Builder
	b.WriteString(m.header())
	if m.lib {
		b.WriteString(m.libView())
		return b.String()
	}
	b.WriteString(m.searchBar())
	b.WriteString(m.list())
	b.WriteString(m.detail())
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) face() []string {
	if m.loading {
		if (time.Now().UnixMilli()/400)%2 == 0 {
			return moodLookLeft
		}
		return moodLookRight
	}
	if time.Now().Before(m.moodUntil) && m.mood != nil {
		return m.mood
	}
	if m.asleep {
		return asleep
	}
	if !m.cfg.Animation {
		return moodStill
	}
	return idle[m.frame].lines
}

func (m Model) header() string {
	info := m.infoLines()
	if !m.showNox() {
		return ansi.Truncate(" "+sBold.Render(app.Name)+" "+sDim.Render(app.Version)+"  "+sDim.Render(info[2]), m.w-1, "…") + "\n\n"
	}
	face := m.face()
	var b strings.Builder
	for i := range face {
		left := sNox.Render(padRight(face[i], noxW+2))
		right := ""
		if i < len(info) {
			right = info[i]
			if m.revealed < revealDone {
				right = ansi.Truncate(right, m.revealed, "")
			}
		}
		b.WriteString(ansi.Truncate(" "+left+right, m.w-1, "…") + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (m Model) infoLines() []string {
	label := func(s string) string { return sDim.Render(padRight(strings.ToUpper(s), 10)) }
	counts := map[string]int{}
	for _, it := range m.res.Items {
		counts[it.Tool]++
	}
	var parts []string
	for _, t := range m.res.Tools {
		parts = append(parts, toolStyle[t].Render(t)+" "+sText.Render(fmt.Sprint(counts[t])))
	}
	sessions := sNeon.Render(fmt.Sprint(len(m.res.Items)))
	if len(parts) > 0 {
		sessions += sDim.Render("  ·  ") + strings.Join(parts, sDim.Render("  ·  "))
	}
	if m.loading {
		sessions = sDim.Render("scanning…")
	}
	lines := []string{
		sBold.Render(app.Name) + " " + sDim.Render(app.Version) + "  " + sDim.Render("· "+app.Mascot+" is watching your sessions"),
		sRule.Render(strings.Repeat("─", 30)),
		label("sessions") + sessions,
	}
	if len(m.res.Items) > 0 {
		last := m.res.Items[0]
		for _, it := range m.res.Items { // most recent, ignoring pins
			if it.Updated.After(last.Updated) {
				last = it
			}
		}
		room := max(10, m.w-noxW-16)
		lines = append(lines,
			label("latest")+sText.Render(ansi.Truncate(last.Label(), room, "…")),
			label("")+sMuted.Render(last.Project())+sDim.Render(join(" · ", last.Branch, ago(last.Updated))),
		)
	}
	if !m.loading {
		lines = append(lines, label("scan")+sDim.Render(m.res.Took.Round(time.Millisecond).String()))
	}
	if acc := m.accountLine(); acc != "" {
		lines = append(lines, label("account")+acc)
	}
	if m.notice != "" {
		lines = append(lines, label("update")+sAccent.Render(m.notice+" available")+sDim.Render("  → jedkx-cli update"))
	}
	for _, t := range m.res.Missing {
		lines = append(lines, label("note")+sWarn.Render(t+" is not on PATH")+sDim.Render(" — sessions listed, cannot open"))
	}
	return lines
}

// accountLine: which account each tool is signed in with, dim ("claude a@b · codex c@d").
func (m Model) accountLine() string {
	var parts []string
	for _, t := range m.res.Tools {
		if e := m.accounts[t]; e != "" {
			parts = append(parts, sDim.Render(t+" "+e))
		}
	}
	return strings.Join(parts, sDim.Render("  ·  "))
}

func (m Model) searchBar() string {
	var left string
	switch {
	case m.renaming:
		left = sAccent.Render(" rename ") + m.rename.View()
	case m.searching || m.search.Value() != "":
		left = sAccent.Render(" / ") + m.search.View()
	default:
		left = sDim.Render(" / search")
	}
	count := sDim.Render(fmt.Sprintf("  %d ", len(m.view)))
	right := m.tabsFit(m.w-ansi.StringWidth(left)-ansi.StringWidth(count)-1) + count
	gap := max(1, m.w-ansi.StringWidth(left)-ansi.StringWidth(right))
	line := ansi.Truncate(left+strings.Repeat(" ", gap)+right, m.w, "")
	return line + "\n" + sRule.Render(strings.Repeat("─", m.w)) + "\n"
}

// tabsFit is tabsBar when it fits in room; otherwise only the current tab and
// where it is ("[codex] 3/5"), so a narrow pane never cuts the tabs mid-word.
func (m Model) tabsFit(room int) string {
	if full := m.tabsBar(); ansi.StringWidth(full) <= room {
		return full
	}
	cur := m.tabIndex()
	short := sAccent.Render("["+tabNames[cur]+"]") + sDim.Render(fmt.Sprintf(" %d/%d", cur+1, len(tabNames)))
	if ansi.StringWidth(short) <= room {
		return short
	}
	return sAccent.Render("[" + tabNames[cur] + "]")
}

// tabsBar shows every tab — chats (all · claude · codex · archived) │ skills & md —
// with the current one in brackets; tab / shift+tab move through them.
func (m Model) tabsBar() string {
	var b strings.Builder
	cur := m.tabIndex()
	for i, name := range tabNames {
		if i == libTab {
			b.WriteString(sRule.Render(" │"))
		}
		if i == cur {
			b.WriteString(sAccent.Render("[" + name + "]"))
		} else {
			b.WriteString(sDim.Render(" " + name + " "))
		}
	}
	return b.String()
}

func (m Model) list() string {
	rows := m.listRows()
	var b strings.Builder
	if len(m.view) == 0 {
		msg := "no sessions yet — run claude or codex once, then come back"
		switch {
		case m.loading:
			msg = "scanning sessions…"
		case len(m.res.Items) > 0:
			msg = "nothing matches"
		}
		b.WriteString(ansi.Truncate("  "+sDim.Render(msg), m.w-1, "…") + "\n")
		rows--
	}
	showProject, showBranch := m.w >= 70 && m.flat, m.w >= 96
	for r := 0; r < rows; r++ {
		c := m.offset + r
		if c >= len(m.rows) {
			b.WriteString("\n")
			continue
		}
		sel := c == m.cursor
		if m.rows[c].item < 0 {
			b.WriteString(m.groupLine(m.rows[c], sel) + "\n")
			continue
		}
		it := m.res.Items[m.rows[c].item]
		indent := ""
		if !m.flat { // tree guides tie the chats to their folder
			last := c+1 >= len(m.rows) || m.rows[c+1].item < 0
			if last {
				indent = sRule.Render("└─")
			} else {
				indent = sRule.Render("├─")
			}
		}

		bar := "  "
		if sel {
			bar = sSelBar.Render("▌ ")
		}
		pin := "  "
		if it.Pinned {
			pin = sAccent.Render("● ")
		}
		if m.marked[it.Key()] { // selected for a bulk action
			pin = sNeon.Render("✓ ")
		}
		if pin == "  " && !m.flat {
			pin = sRule.Render("─ ") // the guide runs on to the chat
		}
		ageSt := sDim
		if time.Since(it.Updated) < time.Hour { // fresh: easy to spot
			ageSt = sNeon
		}
		age := ageSt.Render(padLeft(ago(it.Updated), 4))
		project, branch := "", ""
		fixed := 2 + ansi.StringWidth(indent) + 2 + 4 + 1
		// the tool's diamond leads the title; its name sits in one aligned column
		mark, tool := "", ""
		if m.w >= 24 {
			mark = toolGlyphOf(it.Tool) + " "
			fixed += 2
		}
		if m.w >= 40 { // narrow side pane: titles matter more than the tool name
			tool = sDim.Render(padRight(it.Tool, 7))
			fixed += 7
		}
		if showProject {
			project = sMuted.Render(padRight(ansi.Truncate(it.Project(), 18, "…"), 19))
			fixed += 19
		}
		if showBranch {
			branch = sDim.Render(padRight(ansi.Truncate(it.Branch, 16, "…"), 17))
			fixed += 17
		}
		tw := max(8, m.w-fixed)
		titleSt := sTitle
		if sel {
			titleSt = sBold
		}
		title := titleSt.Render(padRight(ansi.Truncate(it.Label(), tw-1, "…"), tw))
		b.WriteString(bar + indent + pin + mark + title + tool + project + branch + age + "\n")
	}
	return b.String()
}

func (m Model) detail() string {
	rule := sRule.Render(strings.Repeat("─", m.w)) + "\n"
	it, ok := m.selected()
	if !ok {
		lines := m.folderInfo()
		for len(lines) < 2+m.previewRows() { // keep the layout still while moving
			lines = append(lines, "")
		}
		return rule + strings.Join(lines, "\n") + "\n"
	}
	w := max(10, m.w-4)
	prompt := it.Prompt
	if prompt == "" || prompt == it.Label() {
		prompt = "—"
	}
	l1 := "  " + sDim.Render("› ") + sMuted.Render(ansi.Truncate(prompt, w-2, "…"))
	l2 := "  " + sDim.Render(ansi.Truncate(join("  ", it.CWD, shortID(it.ID), it.Updated.Format("2006-01-02 15:04"), strings.Join(it.Tags, " ")), w, "…"))
	if !m.previewOn {
		return rule + l1 + "\n" + l2 + "\n"
	}
	// preview: the latest messages, newest at the bottom, one line each
	n := m.previewRows() + 1
	turns := m.preview
	if len(turns) > n {
		turns = turns[len(turns)-n:]
	}
	var b strings.Builder
	b.WriteString(rule + l2 + "\n")
	for i := 0; i < n; i++ {
		if i >= len(turns) {
			if i == 0 {
				b.WriteString("  " + sDim.Render("no messages to show") + "\n")
			} else {
				b.WriteString("\n")
			}
			continue
		}
		t := turns[i]
		who, st := sAccent.Render("you › "), sTitle
		if !t.User {
			who, st = sNeon.Render("ai  ‹ "), sMuted
		}
		b.WriteString("  " + who + st.Render(ansi.Truncate(t.Text, w-6, "…")) + "\n")
	}
	return b.String()
}

// folderInfo describes the folder header under the cursor: where it is, how
// many chats from which tools, and when it was last used.
func (m Model) folderInfo() []string {
	g, ok := m.onGroup()
	if !ok {
		return nil
	}
	w := max(10, m.w-4)
	items := m.groupItems(g)
	where := "your pinned chats"
	tools := map[string]int{}
	var latest time.Time
	for _, i := range items {
		it := m.res.Items[i]
		if g != pinnedGroup && where == "your pinned chats" && it.CWD != "" {
			where = it.CWD
		}
		tools[it.Tool]++
		if it.Updated.After(latest) {
			latest = it.Updated
		}
	}
	noun := "chats"
	if len(items) == 1 {
		noun = "chat"
	}
	parts := []string{fmt.Sprintf("%d %s", len(items), noun)}
	for _, t := range toolFilters[1:] {
		if n := tools[t]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", t, n))
		}
	}
	if !latest.IsZero() {
		parts = append(parts, "latest "+ago(latest)+" ago")
	}
	l1 := "  " + sAccent.Render("▸ ") + sMuted.Render(ansi.Truncate(where, w-2, "…"))
	l2 := "  " + sDim.Render(ansi.Truncate(strings.Join(parts, " · "), w, "…"))
	return []string{l1, l2}
}

// footer is two lines: what you can do right now, in plain words. Every line is
// cut to the width, and "? help" is always there, however narrow the panel.
func (m Model) footer() string {
	w := max(1, m.w-1)
	l1, l2 := m.footerLines()
	return "\n" + ansi.Truncate(" "+l1, w, "…") + "\n" + ansi.Truncate(" "+l2, w, "…")
}

func (m Model) footerLines() (string, string) {
	switch {
	case m.deleting, m.status != "":
		st := sOK
		if m.statusBad {
			st = sAccent
		}
		return st.Render(m.status), ""
	case m.renaming:
		return keys("enter", "save", "esc", "cancel"), sDim.Render("empty name = original title")
	case m.searching:
		return keys("enter", "open chat", "esc", "stop searching"), keys("ctrl+f", "search inside chats", "↑ ↓", "move")
	}
	// narrow side panel: the first line says how to resize it and move between
	// panes, the second keeps the most useful keys and "? help"
	if pane := paneKeys(); pane != "" && m.w < 70 {
		return pane, keysWrap(m.w-2, m.hints(), 1)[0]
	}
	if n := len(m.markedKeys()); n > 0 {
		return sNeon.Render(fmt.Sprintf("%d selected", n)) + sDim.Render("  ") + keys("d", "delete", "a", "archive", "p", "pin", "e", "export"),
			keysWrap(m.w-2, []string{"x", "select / unselect", "ctrl+a", "all", "esc", "clear"}, 1)[0]
	}
	lines := keysWrap(m.w-2, m.hints(), 2)
	return lines[0], lines[1]
}

// paneKeys are the terminal's own keys for panes ("" when there are no panes).
func paneKeys() string {
	switch launch.Host() {
	case launch.HostWindowsTerminal:
		return keys("alt+shift+←→", "resize", "alt+←→", "switch")
	case launch.HostTmux:
		return keys("ctrl+b ←→", "switch", "ctrl+b alt+←→", "resize")
	}
	return ""
}

// hints are the keys that make sense for what is selected, most useful first,
// labelled in everyday words.
func (m Model) hints() []string {
	if g, ok := m.onGroup(); ok {
		action := "close group"
		if m.folded[g] {
			action = "open group"
		}
		return []string{"enter", action, "z", "close all", "/", "search", "v", "plain list", "q", "quit"}
	}
	if len(m.rows) == 0 {
		return []string{"n", "new chat", "ctrl+r", "look again", "q", "quit"}
	}
	h := []string{"enter", "open chat", "/", "search"}
	if m.flat {
		h = append(h, "v", "group by project")
	} else {
		h = append(h, "←", "close group")
	}
	return append(h, "ctrl+r", "refresh", "space", "preview", "t", "new tab", "n", "new chat", "c", "copy command", "e", "export", "tab", "skills & md", "d", "delete", "s", "sort", "f", "folder", "p", "pin", "o", "full screen", "q", "quit")
}

// keysWrap lays key hints over up to n lines; the last line ends with "? help".
func keysWrap(w int, pairs []string, n int) []string {
	lines := make([]string, n)
	more := keys("?", "help")
	l := 0
	for i := 0; i+1 < len(pairs) && l < n; i += 2 {
		item := keys(pairs[i], pairs[i+1])
		for l < n {
			next := item
			if lines[l] != "" {
				next = lines[l] + sDim.Render("  ") + item
			}
			room := w
			if l == n-1 {
				room -= ansi.StringWidth(more) + 2
			}
			if ansi.StringWidth(next) <= room {
				lines[l] = next
				break
			}
			l++
		}
	}
	last := n - 1
	if lines[last] != "" {
		lines[last] += sDim.Render("  ")
	}
	lines[last] += more
	return lines
}

func (m Model) helpView() string {
	sections := []struct {
		title string
		rows  [][2]string
	}{
		{"open", [][2]string{
			{"enter", "open beside the list (resize: alt+shift+← →)"},
			{"o", "open here, full screen"},
			{"t", "open in a new tab"},
			{"n", "new chat in the same folder, in a new tab"},
			{"space", "preview: the last messages of the chat"},
			{"c", "copy the resume command (paste it in any terminal)"},
			{"e", "export the whole chat as Markdown (Downloads)"},
			{"tab", "all · claude · codex · archived │ skills & md"},
		}},
		{"select", [][2]string{
			{"x", "select a chat — on a project: the whole folder"},
			{"ctrl+a", "select everything in view (again: clear)"},
			{"d a p e", "delete · archive · pin · export the selected"},
			{"esc", "clear the selection"},
			{"f", "open the project folder"},
		}},
		{"panes (windows terminal)", [][2]string{
			{"alt+shift+← →", "make this panel wider / narrower"},
			{"alt+← →", "jump between the list and a chat"},
			{"ctrl+shift+w", "close the focused pane"},
		}},
		{"list", [][2]string{
			{"↑ ↓", "move  (pgup pgdn g G jump)"},
			{"← →", "fold / unfold the project group"},
			{"z", "fold or unfold all groups"},
			{"v", "group by project ⇄ flat list"},
			{"s", "sort: recent → name → project → size"},
			{"/", "search · esc clears"},
			{"ctrl+f", "while searching: look inside every chat's messages"},
		}},
		{"session", [][2]string{
			{"p", "pin"},
			{"r", "rename (only here; tool files untouched)"},
			{"a", "archive · the archived tab shows them (a again: back)"},
			{"d", "delete for good (asks y/n)"},
		}},
		{"other", [][2]string{
			{"ctrl+r", "refresh (also automatic when you come back to the list)"},
			{"q esc", "quit"},
		}},
	}
	kw := 7
	var b strings.Builder
	line := func(s string) { b.WriteString(ansi.Truncate(s, max(1, m.w-1), "…") + "\n") }
	line(" " + sBold.Render(app.Name) + sDim.Render("  keys"))
	for _, s := range sections {
		line("")
		line(" " + sDim.Render(strings.ToUpper(s.title)))
		for _, r := range s.rows {
			line("  " + sKey.Render(padRight(r[0], kw)) + sMuted.Render(r[1]))
		}
	}
	line("")
	line(" " + sDim.Render("? or esc to go back"))
	return strings.TrimSuffix(b.String(), "\n")
}

// ---------------------------------------------------------------- helpers

func keys(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, sKey.Render(pairs[i])+" "+sDim.Render(pairs[i+1]))
	}
	return strings.Join(parts, sDim.Render("  "))
}

func padRight(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func padLeft(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

func join(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	s := strings.Join(out, sep)
	if s != "" && sep == " · " {
		return sep + s
	}
	return s
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/24/7))
	}
	return t.Format("2006")
}

// beside opens next to the list when the terminal can split (Windows Terminal, tmux), else here.
