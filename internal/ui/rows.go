// What the list shows: filtering, selection, and project groups that fold.

package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/meta"
)

// ---------------------------------------------------------------- selection

func (m *Model) refilter() {
	q := m.search.Value()
	tool := toolFilters[m.tool]
	m.view = m.view[:0]
	for i, it := range m.res.Items {
		if it.Archived != m.archived { // normal view hides archived; the archived tab shows only them
			continue
		}
		if tool != "" && it.Tool != tool {
			continue
		}
		if !catalog.Matches(it, q) && !(q != "" && strings.TrimSpace(q) == m.deepQ && m.deep[it.Key()]) {
			continue
		}
		m.view = append(m.view, i)
	}
	m.sortView()
	m.buildRows()
	m.clamp()
	if _, ok := m.onGroup(); ok && m.search.Value() != "" {
		m.firstSession() // searching: land on a match, not a header
	}
}

// sortView orders the filtered sessions by the chosen mode; pins stay first.
func (m *Model) sortView() {
	items := m.res.Items
	mode := sortModes[m.sortBy]
	sort.SliceStable(m.view, func(a, b int) bool {
		x, y := items[m.view[a]], items[m.view[b]]
		if x.Pinned != y.Pinned {
			return x.Pinned
		}
		switch mode {
		case "name":
			return catalog.Fold(x.Label()) < catalog.Fold(y.Label())
		case "project":
			if px, py := catalog.Fold(x.Project()), catalog.Fold(y.Project()); px != py {
				return px < py
			}
		case "size":
			if x.Size != y.Size {
				return x.Size > y.Size
			}
		}
		return x.Updated.After(y.Updated)
	})
}

func (m *Model) clamp() {
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	rows := m.listRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if rows > 0 && m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m Model) selectedIndex() (int, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) || m.rows[m.cursor].item < 0 {
		return 0, false
	}
	return m.rows[m.cursor].item, true
}

func (m Model) selected() (catalog.Item, bool) {
	i, ok := m.selectedIndex()
	if !ok {
		return catalog.Item{}, false
	}
	return m.res.Items[i], true
}

func (m Model) selectedKey() string {
	if it, ok := m.selected(); ok {
		return it.Key()
	}
	return ""
}

func (m *Model) selectKey(key string) {
	if key == "" {
		return
	}
	for c, r := range m.rows {
		if r.item >= 0 && m.res.Items[r.item].Key() == key {
			m.cursor = c
			m.clamp()
			return
		}
	}
}

// ---------------------------------------------------------------- layout

func (m Model) showNox() bool { return m.h >= 26 && m.w >= 72 }

func (m Model) headerRows() int {
	if m.showNox() {
		return len(moodStill) + 1
	}
	return 2
}

// previewRows are the extra lines the preview takes below the list.
func (m Model) previewRows() int {
	if !m.previewOn {
		return 0
	}
	return min(10, max(3, m.h/3))
}

// listRows = height − header − search(2) − detail(3) − footer(3)
func (m Model) listRows() int {
	if m.h == 0 {
		return 10
	}
	return max(1, m.h-m.headerRows()-2-3-3-m.previewRows()) // footer: blank + two hint lines
}

// ---------------------------------------------------------------- groups

// row is one list line: a project group header (item < 0) or a session.
type row struct {
	group string
	item  int
	n     int // header: sessions in the group
}

const pinnedGroup = "pinned"

func groupOf(it catalog.Item) string {
	if it.Pinned {
		return pinnedGroup
	}
	if k := it.ProjectKey(); k != "" {
		return k
	}
	return "—"
}

// groupLabels names each group by its folder; folders with the same name get
// their parent too ("work/api", "old/api").
func groupLabels(order []string, members map[string][]int, items []catalog.Item) map[string]string {
	labels := map[string]string{}
	seen := map[string]int{}
	for _, g := range order {
		if g == pinnedGroup || len(members[g]) == 0 {
			continue
		}
		labels[g] = items[members[g][0]].Project()
		seen[strings.ToLower(labels[g])]++
	}
	for _, g := range order {
		l, ok := labels[g]
		if !ok || seen[strings.ToLower(l)] < 2 {
			continue
		}
		cwd := filepath.Clean(items[members[g][0]].CWD)
		labels[g] = filepath.Base(filepath.Dir(cwd)) + "/" + l
	}
	return labels
}

// buildRows groups the filtered sessions by project, in list order (pinned first,
// then by most recent session). A search shows every match, folded or not.
func (m *Model) buildRows() {
	m.rows = m.rows[:0]
	if m.flat {
		for _, i := range m.view {
			m.rows = append(m.rows, row{item: i})
		}
		return
	}
	var order []string
	members := map[string][]int{}
	for _, i := range m.view {
		g := groupOf(m.res.Items[i])
		if _, ok := members[g]; !ok {
			order = append(order, g)
		}
		members[g] = append(members[g], i)
	}
	m.labels = groupLabels(order, members, m.res.Items)
	open := m.search.Value() != ""
	for _, g := range order {
		m.rows = append(m.rows, row{group: g, item: -1, n: len(members[g])})
		if open || !m.folded[g] {
			for _, i := range members[g] {
				m.rows = append(m.rows, row{group: g, item: i})
			}
		}
	}
}

func (m Model) onGroup() (string, bool) {
	if m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].item < 0 {
		return m.rows[m.cursor].group, true
	}
	return "", false
}

func (m Model) currentGroup() string {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return m.rows[m.cursor].group
	}
	return ""
}

// fold collapses or expands g and keeps the cursor on its header when folding.
func (m *Model) fold(g string, closed bool) {
	if m.flat || g == "" || m.search.Value() != "" { // search shows every match
		return
	}
	if m.folded == nil {
		m.folded = map[string]bool{}
	}
	m.folded[g] = closed
	keep := m.selectedKey()
	m.buildRows()
	if closed {
		keep = ""
		for c, r := range m.rows {
			if r.item < 0 && r.group == g {
				m.cursor = c
			}
		}
	}
	m.selectKey(keep)
	m.clamp()
}

func (m *Model) foldAll() {
	if m.flat {
		return
	}
	all := true
	for _, r := range m.rows {
		if r.item < 0 && !m.folded[r.group] {
			all = false
		}
	}
	if m.folded == nil {
		m.folded = map[string]bool{}
	}
	g := m.currentGroup()
	for _, r := range m.rows {
		if r.item < 0 {
			m.folded[r.group] = !all
		}
	}
	m.buildRows()
	for c, r := range m.rows {
		if r.item < 0 && r.group == g {
			m.cursor = c
		}
	}
	m.clamp()
}

func (m Model) groupLine(r row, sel bool) string {
	bar := "  "
	if sel {
		bar = sSelBar.Render("▌ ")
	}
	arrow := "▾ "
	if m.folded[r.group] && m.search.Value() == "" {
		arrow = "▸ "
	}
	name := strings.ToUpper(m.labels[r.group])
	if name == "" {
		name = strings.ToUpper(r.group)
	}
	st := sNeon
	if sel {
		st = sNeon.Bold(true)
	}
	if r.group == pinnedGroup {
		name = "● PINNED"
	}
	picked, latest := 0, time.Time{}
	for _, i := range m.groupItems(r.group) {
		if m.marked[m.res.Items[i].Key()] {
			picked++
		}
		if u := m.res.Items[i].Updated; u.After(latest) {
			latest = u
		}
	}
	right := sMuted.Render(fmt.Sprint(r.n))
	if m.w >= 50 {
		right += sDim.Render(" · " + ago(latest))
	}
	if picked > 0 {
		right = sNeon.Render(fmt.Sprintf("✓%d  ", picked)) + right
	}
	left := bar + sAccent.Render(arrow) + st.Render(ansi.Truncate(name, max(4, m.w-22), "…")) + " "
	// a faint rule runs from the name to the numbers, like a section divider
	fill := m.w - 1 - ansi.StringWidth(left) - ansi.StringWidth(right) - 1
	if fill < 2 {
		return ansi.Truncate(left+" "+right, m.w-1, "")
	}
	return left + sRule.Render(strings.Repeat("─", fill)) + " " + right
}

// firstSession puts the cursor on the first session instead of its group header.
func (m *Model) firstSession() {
	for c, r := range m.rows {
		if r.item >= 0 {
			m.cursor = c
			m.clamp()
			return
		}
	}
}

// ---------------------------------------------------------------- selection (x / X)

func (m *Model) toggleMark() {
	if m.marked == nil {
		m.marked = map[string]bool{}
	}
	if g, ok := m.onGroup(); ok { // a header selects / clears the whole folder
		keys := m.groupKeys(g)
		all := true
		for _, k := range keys {
			all = all && m.marked[k]
		}
		for _, k := range keys {
			if all {
				delete(m.marked, k)
			} else {
				m.marked[k] = true
			}
		}
		return
	}
	if it, ok := m.selected(); ok {
		if m.marked[it.Key()] {
			delete(m.marked, it.Key())
		} else {
			m.marked[it.Key()] = true
		} // the cursor stays where it is
	}
}

func (m *Model) markAll() {
	if m.marked == nil {
		m.marked = map[string]bool{}
	}
	all := len(m.view) > 0
	for _, i := range m.view {
		all = all && m.marked[m.res.Items[i].Key()]
	}
	for _, i := range m.view {
		if all {
			delete(m.marked, m.res.Items[i].Key())
		} else {
			m.marked[m.res.Items[i].Key()] = true
		}
	}
}

// groupKeys are the keys of the filtered chats in group g (folded or not).
func (m Model) groupKeys(g string) []string {
	var out []string
	for _, i := range m.groupItems(g) {
		out = append(out, m.res.Items[i].Key())
	}
	return out
}

// groupItems are the indexes (into res.Items) of the chats of g in view.
func (m Model) groupItems(g string) []int {
	var out []int
	for _, i := range m.view {
		if groupOf(m.res.Items[i]) == g {
			out = append(out, i)
		}
	}
	return out
}

// markedKeys are the selected chats that still exist and are in view.
func (m Model) markedKeys() []string {
	var out []string
	for _, i := range m.view {
		if k := m.res.Items[i].Key(); m.marked[k] {
			out = append(out, k)
		}
	}
	return out
}

// bulkEdit applies fn to every selected chat, then clears the selection.
func (m *Model) bulkEdit(fn func(*meta.Entry)) int {
	keys := m.markedKeys()
	if err := m.store.UpdateMany(keys, fn); err != nil {
		m.say("save failed: "+err.Error(), true)
		return len(keys)
	}
	for i := range m.res.Items {
		if slices.Contains(keys, m.res.Items[i].Key()) {
			m.res.Items[i].Entry = m.store.Get(m.res.Items[i].Key())
		}
	}
	if len(keys) > 0 {
		m.marked = nil
		catalog.Sort(m.res.Items)
		m.refilter()
	}
	return len(keys)
}
