package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/library"
	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/source"
)

func fakeResult() catalog.Result {
	now := time.Now()
	return catalog.Result{
		Tools: []string{"claude", "codex"},
		Items: []catalog.Item{
			{Session: source.Session{Tool: "claude", ID: "b7f5b17b-0000", Title: "Compare test and prod config", CWD: `C:\dev\shop`, Updated: now}},
			{Session: source.Session{Tool: "codex", ID: "01a17234-0000", Title: "Script task", CWD: `C:\proj`, Branch: "dev", Updated: now.Add(-time.Hour)}, Entry: meta.Entry{Pinned: true}},
		},
	}
}

// Every line must fit the terminal at any size — no wrapping, no overflow.
func TestViewFits(t *testing.T) {
	for _, size := range [][2]int{{220, 60}, {140, 40}, {100, 30}, {80, 20}, {50, 12}, {40, 30}, {30, 8}, {24, 30}, {18, 12}} {
		var m tea.Model = New(config.Default())
		m, _ = m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m, _ = m.Update(loadedMsg(fakeResult()))
		out := m.View()
		lines := strings.Split(out, "\n")
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > size[0] {
				t.Errorf("%dx%d line %d is %d wide: %q", size[0], size[1], i, w, ansi.Strip(l))
			}
		}
		if s := ansi.Strip(out); !strings.Contains(s, "Compare") && !strings.Contains(s, "Script") {
			t.Errorf("%dx%d: session missing:\n%s", size[0], size[1], s)
		}
	}
}

// text-reveal must end by itself (no ticker left running) and any key must skip it.
func TestRevealStops(t *testing.T) {
	var m tea.Model = New(config.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	m, _ = m.Update(loadedMsg(fakeResult()))
	var cmd tea.Cmd = revealTick()
	for i := 0; cmd != nil; i++ {
		if i > 500 {
			t.Fatal("reveal never stopped")
		}
		m, cmd = m.Update(revealMsg{})
	}
	if m.(Model).revealed != revealDone {
		t.Fatal("reveal not marked done")
	}

	var s tea.Model = New(config.Default())
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	if s.(Model).revealed != revealDone {
		t.Fatal("a key press must skip the reveal")
	}
}

func TestArchiveFilter(t *testing.T) {
	var m tea.Model = New(config.Default())
	res := fakeResult()
	res.Items[0].Archived = true
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(loadedMsg(res))
	if got := len(m.(Model).view); got != 1 {
		t.Fatalf("archived must be hidden, view has %d", got)
	}
	m, _ = m.(Model).setTab(archivedTab)
	if got := len(m.(Model).view); got != 1 || m.(Model).res.Items[m.(Model).view[0]].ID != "b7f5b17b-0000" {
		t.Fatal("the archived tab must show only archived")
	}
	// shortcuts ignore case: H is h (fold), not a hidden mode
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Z")})
	if !m.(Model).folded[m.(Model).rows[0].group] {
		t.Fatal("Z must work like z")
	}
}

// Nox follows the portfolio's scheduler: rest 2.2–4.6 s, blink; search after
// 6.5 s and sleep after 22 s without a key, one cue per 14 s; a key wakes it.
func TestNoxSchedule(t *testing.T) {
	for range 200 {
		if d := idleWait(); d < 2200*time.Millisecond || d > 4600*time.Millisecond {
			t.Fatalf("rest %v out of range", d)
		}
	}
	for _, c := range []struct {
		quiet, cool time.Duration
		want        *gesture
	}{
		{time.Second, time.Minute, &blink},
		{7 * time.Second, time.Minute, &search},
		{23 * time.Second, time.Minute, &sleepy},
		{time.Minute, 5 * time.Second, &blink}, // cooldown: only blinks
	} {
		if g, _ := nextGesture(c.quiet, c.cool); &g.steps[0] != &c.want.steps[0] {
			t.Errorf("quiet %v cool %v: wrong gesture", c.quiet, c.cool)
		}
	}
	for _, g := range []gesture{blink, search, sleepy, wake, notice, surprised} {
		for _, s := range g.steps {
			if s.d < 50*time.Millisecond || s.d > 1300*time.Millisecond {
				t.Fatalf("frame %v outside the sheet's durations", s.d)
			}
		}
	}
	// the blink never shuts both eyes at once (residentBlink)
	for _, s := range blink.steps {
		if strings.Join(s.lines, "") == strings.Join(pose(eyeClosed, eyeClosed, mouthSmile), "") {
			t.Fatal("blink closed both eyes together")
		}
	}

	var tm tea.Model = New(config.Default())
	tm, _ = tm.Update(loadedMsg(fakeResult())) // while scanning Nox looks around instead
	tm, _ = tm.Update(sleepMsg{at: time.Now().Add(time.Second)})
	if !tm.(Model).asleep || strings.Join(tm.(Model).face(), "") != strings.Join(asleep, "") {
		t.Fatal("must sleep with eyes shut")
	}
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyDown})
	if tm.(Model).asleep {
		t.Fatal("a key must wake Nox")
	}
}

func TestDeleteNeedsYes(t *testing.T) {
	var m tea.Model = New(config.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(loadedMsg(fakeResult()))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !m.(Model).deleting {
		t.Fatal("d must ask first")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.(Model).deleting || m.(Model).status != "kept" {
		t.Fatal("n must keep the session")
	}
}

func TestGroupsFold(t *testing.T) {
	var m tea.Model = New(config.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(loadedMsg(fakeResult()))
	// pinned codex + workspace claude → 2 headers + 2 sessions
	if got := len(m.(Model).rows); got != 4 {
		t.Fatalf("rows = %d, want 4", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	if got := len(m.(Model).rows); got != 2 {
		t.Fatalf("z must fold all, rows = %d", got)
	}
	if _, ok := m.(Model).selected(); ok {
		t.Fatal("cursor should sit on a header after folding")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := len(m.(Model).rows); got != 3 {
		t.Fatalf("enter on a header must unfold it, rows = %d", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if got := len(m.(Model).rows); got != 2 {
		t.Fatalf("v must flatten, rows = %d", got)
	}
}

// Help and footer must fit any width too; the footer always offers "? keys".
func TestHelpAndHintsFit(t *testing.T) {
	for _, w := range []int{18, 24, 40, 80, 160} {
		var m tea.Model = New(config.Default())
		m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		m, _ = m.Update(loadedMsg(fakeResult()))
		foot := ansi.Strip(m.(Model).footer())
		if !strings.Contains(foot, "?") {
			t.Errorf("w=%d footer lost the help key: %q", w, foot)
		}
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
		for i, l := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(l) > w {
				t.Errorf("w=%d help line %d too wide: %q", w, i, ansi.Strip(l))
			}
		}
	}
}

func TestLibraryFits(t *testing.T) {
	for _, w := range []int{18, 40, 80, 160} {
		m := New(config.Default())
		m.w, m.h, m.lib = w, 30, true
		m.libItems = []library.Entry{{Kind: "instructions", Tool: "claude", Scope: "global", Name: strings.Repeat("long-name-", 8), Desc: strings.Repeat("words ", 40), Path: `C:\x\CLAUDE.md`}}
		for i, l := range strings.Split(m.View(), "\n") {
			if ansi.StringWidth(l) > w {
				t.Errorf("w=%d library line %d too wide: %q", w, i, ansi.Strip(l))
			}
		}
	}
}

func TestSelectFolderAndAll(t *testing.T) {
	var tm tea.Model = New(config.Default())
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	tm, _ = tm.Update(loadedMsg(fakeResult()))
	m := tm.(Model)
	m.cursor = 0 // first group header
	g, ok := m.onGroup()
	if !ok {
		t.Fatal("expected a header at row 0")
	}
	m.toggleMark()
	if len(m.markedKeys()) != len(m.groupKeys(g)) || len(m.markedKeys()) == 0 {
		t.Fatalf("header x must select the folder: %d of %d", len(m.markedKeys()), len(m.groupKeys(g)))
	}
	m.markAll()
	if len(m.markedKeys()) != len(m.view) {
		t.Fatal("X must select everything in view")
	}
	m.markAll()
	if len(m.markedKeys()) != 0 {
		t.Fatal("X again must clear")
	}
}

func TestTabsReachLibrary(t *testing.T) {
	var tm tea.Model = New(config.Default())
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	tm, _ = tm.Update(loadedMsg(fakeResult()))
	tab := tea.KeyMsg{Type: tea.KeyTab}
	for range 3 {
		tm, _ = tm.Update(tab)
	}
	if !tm.(Model).archived {
		t.Fatal("4th tab must be archived")
	}
	tm, _ = tm.Update(tab)
	m := tm.(Model)
	if !m.lib || m.archived {
		t.Fatal("5th tab must be the library")
	}
	m.libItems = []library.Entry{{Kind: "skill", Name: "a"}, {Kind: "instructions", Name: "CLAUDE"}}
	if len(m.libShown()) != 2 {
		t.Fatal("the library shows skills and md files together")
	}
	tm, _ = m.Update(tab)
	if tm.(Model).lib || tm.(Model).archived || tm.(Model).tool != 0 {
		t.Fatal("tab after the library goes back to all chats")
	}
}

func TestStatusClears(t *testing.T) {
	var tm tea.Model = New(config.Default())
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	tm, _ = tm.Update(loadedMsg(fakeResult()))
	tm, _ = tm.Update(exportedMsg{path: "x.md"})
	m := tm.(Model)
	if m.status == "" {
		t.Fatal("status expected")
	}
	tm, _ = m.Update(statusClearMsg{m.statusSeq - 1})
	if tm.(Model).status == "" {
		t.Fatal("an old timer must not clear a newer message")
	}
	tm, _ = tm.Update(statusClearMsg{m.statusSeq})
	if tm.(Model).status != "" {
		t.Fatal("status must clear after its time")
	}
}

// On a folder header the bottom shows the folder, never an empty preview.
func TestFolderInfoOnHeader(t *testing.T) {
	var tm tea.Model = New(config.Default())
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	tm, _ = tm.Update(loadedMsg(fakeResult()))
	m := tm.(Model)
	m.previewOn, m.cursor = true, 0
	out := ansi.Strip(m.View())
	if strings.Contains(out, "no messages to show") || !strings.Contains(out, "1 chat") {
		t.Fatalf("folder header must show the folder:\n%s", out)
	}
}

// A narrow pane never cuts a tab name in half: all tabs fit, or only the current one shows.
func TestTabsNeverCut(t *testing.T) {
	for w := 18; w <= 130; w += 4 {
		var tm tea.Model = New(config.Default())
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		tm, _ = tm.Update(loadedMsg(fakeResult()))
		bar := strings.SplitN(ansi.Strip(tm.(Model).searchBar()), "\n", 2)[0]
		bar = strings.TrimPrefix(bar, " / search") // "search" itself contains "arch"
		if strings.Contains(bar, "arch") && !strings.Contains(bar, "archived") ||
			strings.Contains(bar, "│ s") && !strings.Contains(bar, "skills & md") {
			t.Errorf("w=%d tab cut: %q", w, bar)
		}
	}
}
