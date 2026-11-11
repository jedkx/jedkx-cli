package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jedkx/jedkx-cli/internal/app"
	"github.com/jedkx/jedkx-cli/internal/config"
)

// Run starts the session manager.
func Run(cfg config.Config) error { return RunQuery(cfg, "") }

// RunQuery opens the list with a search already typed (e.g. this folder).
func RunQuery(cfg config.Config, q string) error {
	m := New(cfg)
	if q != "" {
		m.search.SetValue(q)
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus()).Run()
	return err
}

// Banner is a static Nox + name block for plain (non-TUI) output.
func Banner(subtitle string) string {
	var b strings.Builder
	for _, l := range moodStill {
		b.WriteString("  " + sNox.Render(l) + "\n")
	}
	b.WriteString("\n  " + sBold.Render(app.Name) + " " + sDim.Render(app.Version))
	if subtitle != "" {
		b.WriteString("  " + sMuted.Render(subtitle))
	}
	return b.String() + "\n"
}

// Accent / Dim style helpers for the CLI layer.
func Accent(s string) string { return sAccent.Render(s) }
func Dim(s string) string    { return sDim.Render(s) }
func OK(s string) string     { return sOK.Render(s) }
func Bold(s string) string   { return sBold.Render(s) }
func Muted(s string) string  { return sMuted.Render(s) }

// Alert is a bold red line the user must not miss.
func Alert(s string) string { return sAccent.Bold(true).Render(s) }

// Rule is a faint horizontal line for the CLI screens.
func Rule(w int) string { return sRule.Render(strings.Repeat("─", w)) }

// ---------------------------------------------------------------- jedkx-cli nox

type noxModel struct {
	w, h           int
	face           []string // nil = at rest
	start, lastCue time.Time
	asleep         bool
}

func (m noxModel) Init() tea.Cmd { return func() tea.Msg { return playMsg{notice} } }

// run plays g and schedules what comes after it, like the list does.
func (m *noxModel) run(g gesture) tea.Cmd {
	m.face = g.steps[0].lines
	var cmds []tea.Cmd
	at := time.Duration(0)
	for i, s := range g.steps {
		at += s.d
		var next []string // after the last step: back to rest
		if i+1 < len(g.steps) {
			next = g.steps[i+1].lines
		}
		cmds = append(cmds, tea.Tick(at, func(time.Time) tea.Msg { return moodStepMsg{next} }))
	}
	if g.sleeps {
		cmds = append(cmds, tea.Tick(at, func(time.Time) tea.Msg { return sleepMsg{} }))
	} else {
		cmds = append(cmds, ambientTick(at))
	}
	return tea.Batch(cmds...)
}

func (m noxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case playMsg:
		m.start, m.lastCue = time.Now(), time.Now()
		return m, m.run(msg.g)
	case ambientMsg:
		g, cue := nextGesture(time.Since(m.start), time.Since(m.lastCue)) // no keys here: quiet since start
		if cue {
			m.lastCue = time.Now()
		}
		return m, m.run(g)
	case sleepMsg:
		m.asleep, m.face = true, asleep
	case moodStepMsg:
		if !m.asleep {
			m.face = msg.face
		}
	case tea.KeyMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m noxModel) View() string {
	face := m.face
	if face == nil {
		face = idle[0].lines
	}
	art := sNox.Render(strings.Join(face, "\n"))
	cap := sDim.Render("hi, I'm " + app.Mascot + " — I live in your terminal.  any key to leave")
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, art+"\n\n"+cap)
}

// RunNox shows Nox full screen.
func RunNox() error {
	_, err := tea.NewProgram(noxModel{}, tea.WithAltScreen()).Run()
	return err
}
