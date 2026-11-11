// Package ui is the terminal interface: Nox up top, sessions below.
package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/jedkx/jedkx-cli/internal/app"
	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/library"
	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/source"
	"github.com/jedkx/jedkx-cli/internal/update"
)

type (
	loadedMsg   catalog.Result
	execDoneMsg struct{ err error }
)

var toolFilters = []string{"", "claude", "codex"}

// tabNames are what tab cycles through: chats (all / per tool, the start tab
// is all), the archived chats, then the library — skills, agents, commands and
// CLAUDE.md / AGENTS.md files together.
var tabNames = []string{"all", "claude", "codex", "archived", "skills & md"}

const (
	archivedTab = 3 // index of "archived" in tabNames
	libTab      = 4 // the library tab
)

// tabIndex is the current tab in tabNames.
func (m Model) tabIndex() int {
	switch {
	case m.lib:
		return libTab
	case m.archived:
		return archivedTab
	}
	return m.tool
}

// setTab switches to tab i (wrapping around); library tabs scan on first use.
func (m Model) setTab(i int) (tea.Model, tea.Cmd) {
	n := len(tabNames)
	i = ((i % n) + n) % n
	if i < libTab {
		m.lib, m.archived, m.tool = false, i == archivedTab, i%archivedTab
		if m.archived {
			m.tool = 0 // archived chats of every tool
		}
		m.refilter()
		return m, nil
	}
	m.lib, m.archived, m.libCursor, m.libOffset = true, false, 0, 0
	if m.libItems == nil {
		m.say("scanning skills and instruction files…", false)
		return m, m.loadLibrary()
	}
	return m, nil
}

// sortModes: s cycles through them (pins stay on top).
var sortModes = []string{"recent", "name", "project", "size"}

// Model is the Bubble Tea model.
type Model struct {
	cfg   config.Config
	store *meta.Store

	w, h    int
	res     catalog.Result
	view    []int // indices into res.Items after filtering
	rows    []row // what the list shows: group headers + visible items (cursor indexes this)
	cursor  int
	flat    bool            // v: plain list instead of project groups
	folded  map[string]bool // collapsed groups
	offset  int
	loading bool

	search    textinput.Model
	searching bool
	rename    textinput.Model
	renaming  bool
	tool      int // index into toolFilters
	archived  bool
	help      bool

	frame     int
	mood      []string
	moodUntil time.Time

	status      string
	statusBad   bool
	statusSeq   int // bumps on every new message; its timer clears only that one
	moodPending bool
	lastKey     time.Time         // last key: Nox searches / dozes after a quiet spell
	lastCue     time.Time         // last search / sleep; at most one every cueCooldown
	asleep      bool              // eyes shut until a key
	clip        *gesture          // a reaction to play (surprised), started by withMood
	deleting    bool              // waiting for y/n on delete
	deleteKey   string            // the session the y/n is about
	deleteKeys  []string          // or, with chats selected, all of them
	marked      map[string]bool   // x / X: chats selected for a bulk action
	labels      map[string]string // group key (full folder path) → name shown
	previewOn   bool              // space: last messages of the selected chat
	preview     []source.Turn
	previewKey  string
	sortBy      int  // index into sortModes
	lib         bool // L: the library (skills, CLAUDE.md, agents…)
	libItems    []library.Entry
	libCursor   int
	libOffset   int
	deepQ       string          // query the inside-chat search ran for
	deep        map[string]bool // chats whose text contains deepQ
	notice      string
	revealed    int // characters of the info block shown so far (text-reveal)
}

// New builds the model; nothing is scanned until Init.
func New(cfg config.Config) Model {
	si := textinput.New()
	si.Prompt = ""
	si.Placeholder = "search title, prompt, project, branch, Jira key…"
	si.PlaceholderStyle = sDim
	si.TextStyle = sText
	si.Cursor.Style = sAccent
	ri := textinput.New()
	ri.Prompt = ""
	ri.TextStyle = sText
	ri.Cursor.Style = sAccent
	ri.CharLimit = 120
	// terminal-caret (jedkx animations): cursor-blink 1s step-end → 0.5s on / 0.5s off
	si.Cursor.BlinkSpeed, ri.Cursor.BlinkSpeed = caretHalfPeriod, caretHalfPeriod
	if os.Getenv("NO_COLOR") != "" {
		cfg.Animation = false
	}
	now := time.Now()
	m := Model{cfg: cfg, store: meta.Load(meta.File()), search: si, rename: ri, loading: true, notice: update.Notice(), lastKey: now, lastCue: now}
	if !cfg.Animation {
		m.revealed = revealDone
	}
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.load()}
	if m.cfg.Animation {
		// like the portfolio: Nox notices you once, then rests and blinks
		cmds = append(cmds, func() tea.Msg { return playMsg{notice} }, revealTick())
	}
	if c := autoUpdate(m.cfg); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

// text-reveal (jedkx animations): the info block types in once, one character per
// 30 ms stagger like J-Portfolio's textReveal, then the ticker stops for good.
const (
	revealStagger   = 30 * time.Millisecond
	revealDone      = 1 << 30
	caretHalfPeriod = 500 * time.Millisecond
)

type revealMsg struct{}

func revealTick() tea.Cmd {
	return tea.Tick(revealStagger, func(time.Time) tea.Msg { return revealMsg{} })
}

func (m Model) load() tea.Cmd {
	store := m.store
	return func() tea.Msg { return loadedMsg(catalog.Load(store, config.Hidden(m.cfg)...)) }
}

type moodEndMsg struct{}

// react shows a short expression; one timer brings Nox back to rest. Surprise
// plays the portfolio's whole "surprised" clip instead of one face.
func (m *Model) react(face []string) {
	if &face[0] == &moodSurprised[0] && m.cfg.Animation {
		m.clip = &surprised
		return
	}
	m.mood = face
	m.moodUntil = time.Now().Add(moodFor)
	m.moodPending = true
}

// play shows g's faces one after another at their own durations, then rest.
func (m *Model) play(g gesture) tea.Cmd {
	m.mood, m.moodUntil = g.steps[0].lines, time.Now().Add(g.length())
	var cmds []tea.Cmd
	at := time.Duration(0)
	for i, s := range g.steps {
		at += s.d
		if i+1 < len(g.steps) {
			face := g.steps[i+1].lines
			cmds = append(cmds, tea.Tick(at, func(time.Time) tea.Msg { return moodStepMsg{face} }))
		}
	}
	cmds = append(cmds, tea.Tick(at, func(time.Time) tea.Msg { return moodEndMsg{} }))
	return tea.Batch(cmds...)
}

// playMsg plays a gesture now (the opening notice).
type playMsg struct{ g gesture }

// sleepMsg: the doze finished; Nox sleeps unless a key came meanwhile.
type sleepMsg struct{ at time.Time }

const moodFor = 900 * time.Millisecond

func withMood(m Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if m.clip != nil {
		g := *m.clip
		m.clip, m.moodPending = nil, false
		return m, tea.Batch(cmd, m.play(g))
	}
	if !m.moodPending {
		return m, cmd
	}
	m.moodPending = false
	return m, tea.Batch(cmd, tea.Tick(moodFor, func(time.Time) tea.Msg { return moodEndMsg{} }))
}

func (m *Model) say(s string, bad bool) { m.status, m.statusBad = s, bad }

// ---------------------------------------------------------------- update

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	mm, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	cmds := []tea.Cmd{cmd}
	if mm.previewOn {
		cmds = append(cmds, mm.syncPreview())
	}
	// a status message stays a few seconds, then the footer shows the keys again
	if mm.status != "" && (mm.status != m.status || mm.statusBad != m.statusBad) {
		mm.statusSeq++
		seq, d := mm.statusSeq, statusFor
		if mm.statusBad {
			d = 2 * statusFor // errors stay longer
		}
		cmds = append(cmds, tea.Tick(d, func(time.Time) tea.Msg { return statusClearMsg{seq} }))
	}
	return mm, tea.Batch(cmds...)
}

// statusFor is how long a status message shows before the keys come back.
const statusFor = 4 * time.Second

type statusClearMsg struct{ seq int }

// syncPreview starts reading the selected chat's last messages when the
// selection changed; the file is read off the UI thread (previewMsg).
func (m *Model) syncPreview() tea.Cmd {
	it, ok := m.selected()
	if !ok {
		m.preview, m.previewKey = nil, ""
		return nil
	}
	if it.Key() == m.previewKey {
		return nil
	}
	key, s := it.Key(), it.Session
	m.previewKey, m.preview = key, nil
	a := catalog.Adapter(it.Tool)
	if a == nil {
		return nil
	}
	return func() tea.Msg { return previewMsg{key, a.Preview(s, 12)} }
}

// previewMsg carries a preview read in the background.
type previewMsg struct {
	key   string
	turns []source.Turn
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clamp()
		return m, nil

	case revealMsg:
		if m.revealed >= revealDone {
			return m, nil
		}
		m.revealed++
		longest := 0
		for _, l := range m.infoLines() {
			longest = max(longest, ansi.StringWidth(l))
		}
		if !m.loading && m.revealed >= longest {
			m.revealed = revealDone
			return m, nil
		}
		return m, revealTick()

	case loadedMsg:
		keep := m.selectedKey()
		m.res, m.loading = catalog.Result(msg), false
		m.previewKey = "" // chats may have grown: read the preview again
		m.refilter()
		m.selectKey(keep)
		if keep == "" {
			m.firstSession()
		}
		if m.lib { // project files depend on the chats: refresh the library
			return m, m.loadLibrary()
		}
		m.libItems = nil // scanned again on the next visit
		return m, nil

	case focusMsg:
		if msg.ok {
			m.say("switched to the tab \""+msg.title+"\"", false)
		} else {
			m.say("already open in the tab \""+msg.title+"\" (could not switch to it)", false)
		}
		return m, nil
	case execDoneMsg:
		if msg.err != nil {
			m.say(msg.err.Error(), true)
			m.react(moodSurprised)
		} else {
			m.say("welcome back", false)
			m.react(moodHappy)
		}
		return withMood(m, m.load())

	case updatedMsg:
		switch {
		case msg.applied:
			m.notice = ""
			m.say("updated to "+msg.tag+" — restart jedkx-cli to use it", false)
			m.react(moodHappy)
		default:
			m.notice = msg.tag // shown in the header with "→ jedkx-cli update"
		}
		return withMood(m, nil)

	case playMsg:
		if !m.cfg.Animation {
			return m, nil
		}
		return m, tea.Batch(m.play(msg.g), ambientTick(msg.g.length()))

	case ambientMsg:
		if !m.cfg.Animation || m.asleep {
			return m, nil // asleep: no timers until a key wakes Nox
		}
		if left := time.Until(m.moodUntil); left > 0 {
			return m, ambientTick(left) // a reaction is showing; rest after it
		}
		now := time.Now()
		g, cue := nextGesture(now.Sub(m.lastKey), now.Sub(m.lastCue))
		if cue {
			m.lastCue = now
		}
		if g.sleeps {
			return m, tea.Batch(m.play(g), tea.Tick(g.length(), func(time.Time) tea.Msg { return sleepMsg{now} }))
		}
		return m, tea.Batch(m.play(g), ambientTick(g.length()))

	case sleepMsg:
		if m.lastKey.Before(msg.at) {
			m.asleep = true
			return m, nil
		}
		return m, ambientTick(0) // a key came while dozing off: stay awake

	case statusClearMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}
		return m, nil

	case previewMsg:
		if msg.key == m.previewKey { // drop it if the cursor moved on meanwhile
			m.preview = msg.turns
		}
		return m, nil

	case libMsg:
		m.libItems = msg
		if m.libCursor >= len(m.libShown()) {
			m.libCursor, m.libOffset = 0, 0
		}
		return m, nil

	case exportedMsg:
		if msg.err != nil {
			m.say("export failed: "+msg.err.Error(), true)
		} else {
			m.say("saved "+msg.path, false)
			m.react(moodHappy)
		}
		return m, nil

	case deepMsg:
		if msg.q != strings.TrimSpace(m.search.Value()) {
			return m, nil // the query changed meanwhile
		}
		m.deepQ, m.deep = msg.q, msg.keys
		m.refilter()
		m.say(fmt.Sprintf("found inside %d chats", len(msg.keys)), false)
		return m, nil

	case moodStepMsg:
		if time.Now().Before(m.moodUntil) {
			m.mood = msg.face
		}
		return m, nil

	case moodEndMsg:
		return m, nil // just redraw: face() falls back to rest

	case tea.KeyMsg:
		m.revealed = revealDone // any key skips the intro
		m.lastKey = time.Now()
		var woke tea.Cmd
		if m.asleep { // a key wakes Nox; the scheduler starts again after
			m.asleep, m.lastCue = false, m.lastKey
			woke = tea.Batch(m.play(wake), ambientTick(wake.length()))
		}
		if !m.searching && !m.renaming && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			msg.Runes = []rune{unicode.ToLower(msg.Runes[0])} // shortcuts ignore caps lock / shift
		}
		var next tea.Model
		var cmd tea.Cmd
		switch {
		case m.lib && !m.help:
			next, cmd = m.updateLib(msg)
		case m.deleting:
			next, cmd = m.confirmDelete(msg)
		case m.renaming:
			next, cmd = m.updateRename(msg)
		case m.searching:
			next, cmd = m.updateSearch(msg)
		default:
			next, cmd = m.updateKeys(msg)
		}
		cmd = tea.Batch(cmd, woke)
		if nm, ok := next.(Model); ok {
			return withMood(nm, cmd)
		}
		return next, cmd
	}
	return m, nil
}

// ---------------------------------------------------------------- self-update

type updatedMsg struct {
	tag     string
	applied bool
}

// autoUpdate checks GitHub on every start (in the background, 3 s budget) and,
// with update.auto on, installs a newer verified release right away. The new
// binary is used from the next start; this run keeps going untouched.
func autoUpdate(cfg config.Config) tea.Cmd {
	if !cfg.UpdateCheck || app.Version == "dev" {
		return nil
	}
	return func() tea.Msg {
		tag, err := update.Check(3 * time.Second)
		if err != nil || !update.Newer(tag, app.Version) {
			return nil
		}
		if cfg.AutoUpdate {
			if got, err := update.Apply(func(string) {}); err == nil {
				return updatedMsg{tag: got, applied: true}
			}
		}
		return updatedMsg{tag: tag}
	}
}

type ambientMsg struct{}

type libMsg []library.Entry

// loadLibrary scans global files and those of every project with chats.
func (m Model) loadLibrary() tea.Cmd {
	seen := map[string]bool{}
	var dirs []string
	for _, it := range m.res.Items {
		if k := it.ProjectKey(); k != "" && !seen[k] {
			seen[k] = true
			dirs = append(dirs, it.CWD)
		}
	}
	return func() tea.Msg { return libMsg(library.Scan(dirs)) }
}

type exportedMsg struct {
	path string
	err  error
}

// deepMsg carries the chats whose text contains q (ctrl+f in search).
type deepMsg struct {
	q    string
	keys map[string]bool
}

// deepSearch reads every chat file, a few at a time, looking for q.
// deepSearch finds chats whose messages (not the log's JSON keys) contain every
// word of q, folded like the list search so "islem" finds "İşlem".
func deepSearch(items []catalog.Item, q string) map[string]bool {
	words := strings.Fields(catalog.Fold(q))
	found := map[string]bool{}
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, it := range items {
		tr, ok := catalog.Adapter(it.Tool).(source.Transcriber)
		if !ok || len(words) == 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(it catalog.Item) {
			defer func() { <-sem; wg.Done() }()
			turns, err := tr.Transcript(it.Session)
			if err != nil {
				return
			}
			var b strings.Builder
			for _, t := range turns {
				b.WriteString(t.Text)
				b.WriteByte('\n')
			}
			hay := catalog.Fold(b.String())
			for _, w := range words {
				if !strings.Contains(hay, w) {
					return
				}
			}
			mu.Lock()
			found[it.Key()] = true
			mu.Unlock()
		}(it)
	}
	wg.Wait()
	return found
}

type moodStepMsg struct{ face []string }

// ambientTick waits for the current gesture (after) and then a rest (idleWait).
func ambientTick(after time.Duration) tea.Cmd {
	return tea.Tick(after+idleWait(), func(time.Time) tea.Msg { return ambientMsg{} })
}
