package ui

import "github.com/charmbracelet/lipgloss"

// Palette from J-Portfolio (tailwind.config.js): void black, terminal white,
// a single red accent. The terminal's own background is respected — never painted.
var (
	cText   = lipgloss.Color("#f5f5f5")
	cMuted  = lipgloss.Color("#bdbdbd")
	cDim    = lipgloss.Color("#6b6b6b")
	cFaint  = lipgloss.Color("#2a2a2a")
	cAccent = lipgloss.Color("#ff3333")
	cOK     = lipgloss.Color("#00ff00") // terminal green
	cWarn   = lipgloss.Color("#ffaa00")
	cNox    = lipgloss.Color("#e8e8eb")
)

var (
	sText   = lipgloss.NewStyle().Foreground(cText)
	sMuted  = lipgloss.NewStyle().Foreground(cMuted)
	sDim    = lipgloss.NewStyle().Foreground(cDim)
	sAccent = lipgloss.NewStyle().Foreground(cAccent)
	sBold   = lipgloss.NewStyle().Foreground(cText).Bold(true)
	sOK     = lipgloss.NewStyle().Foreground(cOK)
	sWarn   = lipgloss.NewStyle().Foreground(cWarn)
	sNox    = lipgloss.NewStyle().Foreground(cNox)
	sKey    = lipgloss.NewStyle().Foreground(cAccent).Bold(true) // keys stand out in hints
	sNeon   = lipgloss.NewStyle().Foreground(cOK)                // group names, fresh sessions
	sTitle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e2e2e2"))

	sSelBar = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sRule   = lipgloss.NewStyle().Foreground(cFaint)

	// a chat's token count darkens toward red as its context fills (compact soon)
	sCtxHalf = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b5a2a")) // dark amber, ≥ 60 %
	sCtxFull = lipgloss.NewStyle().Foreground(lipgloss.Color("#7a2a2a")) // dark red, ≥ 80 %

	// monochrome like the portfolio: red stays reserved for focus and warnings
	toolStyle = map[string]lipgloss.Style{
		"claude": lipgloss.NewStyle().Foreground(cText),
		"codex":  lipgloss.NewStyle().Foreground(cMuted),
	}
)

// toolGlyph marks the tool at a glance: a filled diamond for Claude Code, an
// outlined one for Codex (monochrome, so red stays for focus).
var toolGlyph = map[string]string{"claude": "◆", "codex": "◇"}

// toolGlyphOf is the tool's diamond in its colour.
func toolGlyphOf(t string) string {
	st, ok := toolStyle[t]
	if !ok {
		st = sMuted
	}
	g := toolGlyph[t]
	if g == "" {
		g = "·"
	}
	return st.Render(g)
}
