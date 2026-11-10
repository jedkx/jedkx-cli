package ui

// Screenshots for the README:
//
//	JEDKX_SHOTS=docs/screenshots go test ./internal/ui -run TestScreenshots -count=1
//
// renders real views with demo data (never your own chats) and writes them as SVG.

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/jedkx/jedkx-cli/internal/app"
	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/library"
	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/source"
	"github.com/jedkx/jedkx-cli/internal/statusline"
)

func demoResult() catalog.Result {
	now := time.Now()
	s := func(tool, id, title, prompt, cwd, branch string, ago time.Duration) catalog.Item {
		return catalog.Item{Session: source.Session{Tool: tool, ID: id, Title: title, Prompt: prompt, CWD: cwd, Branch: branch, Updated: now.Add(-ago), Size: int64(len(title)) * 9000}}
	}
	items := []catalog.Item{
		s("claude", "4f1c2a9e-1", "Refund flow: idempotency keys", "add idempotency keys to POST /refunds", `C:\dev\payments-api`, "feature/PAY-142-refunds", 4*time.Minute),
		s("claude", "4f1c2a9e-2", "Flaky webhook retry test", "the retry test fails one run in ten", `C:\dev\payments-api`, "main", 3*time.Hour),
		s("codex", "9b77e0d1-3", "Migrate settings page to new form lib", "move settings to the new form library", `C:\dev\web-app`, "feature/WEB-88-forms", 40*time.Minute),
		s("claude", "9b77e0d1-4", "Dark mode tokens", "", `C:\dev\web-app`, "main", 26*time.Hour),
		s("claude", "c03d5e7a-5", "Terraform: split prod and staging state", "split the state file", `C:\dev\infra`, "main", 2*24*time.Hour),
		s("codex", "c03d5e7a-6", "Cost report for the k8s cluster", "", `C:\dev\infra`, "ops/cost", 5*24*time.Hour),
		s("claude", "e8a1b2c3-7", "Release notes for 2.4", "draft release notes from the merged PRs", `C:\dev\docs-site`, "main", 9*24*time.Hour),
	}
	items[0].Entry = meta.Entry{Pinned: true}
	catalog.Sort(items)
	return catalog.Result{Tools: []string{"claude", "codex"}, Items: items, Took: 38 * time.Millisecond}
}

func shot(w, h int, setup func(*Model)) string {
	var tm tea.Model = New(config.Default())
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: w, Height: h})
	tm, _ = tm.Update(loadedMsg(demoResult()))
	m := tm.(Model)
	m.revealed = revealDone
	m.notice = ""
	m.res.Missing = nil
	if setup != nil {
		setup(&m)
	}
	return m.View()
}

func TestScreenshots(t *testing.T) {
	dir := os.Getenv("JEDKX_SHOTS")
	if dir == "" {
		t.Skip("set JEDKX_SHOTS=<dir> to write the README screenshots")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Setenv("WT_SESSION", "shots") // panes shown as in Windows Terminal
	t.Setenv("TMUX", "")
	defer func(v string) { app.Version = v }(app.Version)
	app.Version = "v0.1.0"
	_ = os.MkdirAll(dir, 0o755)
	write := func(name, title, text string, cols, rows int) {
		if err := os.WriteFile(filepath.Join(dir, name+".svg"), []byte(toSVG(title, text, cols, rows)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const W, H = 104, 30
	write("list", "jedkx-cli", shot(W, H, nil), W, H)
	write("preview", "jedkx-cli · preview", shot(W, 38, func(m *Model) {
		m.previewOn = true
		m.preview = []source.Turn{
			{User: true, Text: "add idempotency keys to POST /refunds"},
			{Text: "I'll store the key with the refund and return the first result for repeats."},
			{User: true, Text: "and the race when two requests land together?"},
			{Text: "A unique index on (merchant_id, key) makes the second insert fail; we read and return it."},
			{User: true, Text: "good, write the migration"},
		}
	}), W, 38)
	write("select", "jedkx-cli · select", shot(W, H, func(m *Model) {
		m.marked = map[string]bool{}
		for _, it := range m.res.Items {
			if strings.Contains(it.CWD, "infra") || strings.Contains(it.CWD, "web-app") {
				m.marked[it.Key()] = true
			}
		}
	}), W, H)
	write("library", "jedkx-cli · library", shot(W, H, func(m *Model) {
		m.lib = true
		m.libItems = []library.Entry{
			{Kind: "instructions", Tool: "claude", Scope: "global", Name: "CLAUDE", Desc: "Short, correct answers. Never push to shared branches.", Path: `C:\Users\you\.claude\CLAUDE.md`},
			{Kind: "skill", Tool: "claude", Scope: "global", Name: "commit-message", Desc: "Reads the diff and drafts a commit message for review.", Path: `C:\Users\you\.claude\skills\commit-message\SKILL.md`},
			{Kind: "skill", Tool: "claude", Scope: "global", Name: "release-notes", Desc: "Turns merged PRs into release notes.", Path: `C:\Users\you\.claude\skills\release-notes\SKILL.md`},
			{Kind: "agent", Tool: "claude", Scope: "global", Name: "reviewer", Desc: "Reviews a diff for bugs before you merge.", Path: `C:\Users\you\.claude\agents\reviewer.md`},
			{Kind: "instructions", Tool: "claude", Scope: "payments-api", Name: "CLAUDE", Desc: "Payments API: money is in minor units, always.", Path: `C:\dev\payments-api\CLAUDE.md`},
			{Kind: "command", Tool: "claude", Scope: "payments-api", Name: "migrate", Desc: "Create and check a database migration.", Path: `C:\dev\payments-api\.claude\commands\migrate.md`},
			{Kind: "instructions", Tool: "codex", Scope: "web-app", Name: "AGENTS", Desc: "Web app: pnpm, strict TypeScript, no default exports.", Path: `C:\dev\web-app\AGENTS.md`},
			{Kind: "prompt", Tool: "codex", Scope: "global", Name: "explain", Desc: "Explain this code to a new teammate.", Path: `C:\Users\you\.codex\prompts\explain.md`},
		}
		m.libCursor = 1
	}), W, H)
	write("help", "jedkx-cli · keys", shot(W, 46, func(m *Model) { m.help = true }), W, 46)
	write("narrow", "beside a chat", shot(36, H, nil), 36, H)

	// the Claude Code status line, from a throwaway repo
	repo := filepath.Join(t.TempDir(), "payments-api")
	_ = os.MkdirAll(repo, 0o755)
	if exec.Command("git", "-C", repo, "init", "-q", "-b", "feature/PAY-142-refunds").Run() == nil {
		_ = os.WriteFile(filepath.Join(repo, "a.txt"), []byte("x"), 0o644)
	}
	var lines []string
	for _, used := range []int{34, 63, 88} {
		raw := fmt.Sprintf(`{"cwd":%q,"model":{"display_name":"Claude Opus 5.5"},"context_window":{"used_percentage":%d},"cost":{"total_duration_ms":%d,"total_lines_added":%d,"total_lines_removed":%d}}`,
			filepath.ToSlash(repo), used, used*21000, used*2, used/3)
		lines = append(lines, " "+statusline.Render(strings.NewReader(raw)))
	}
	write("statusline", "Claude Code · status line", strings.Join(lines, "\n\n"), 84, 5)
}

// ---------------------------------------------------------------- ANSI → SVG

const (
	cw, lh, pad, bar = 8.4, 18.0, 16.0, 30.0
	bg               = "#020202"
	fgDefault        = "#f5f5f5"
)

type run struct {
	text string
	fg   string
	bold bool
}

// toSVG draws the text as a terminal window. Each styled run is placed at its own
// column, so alignment never depends on the viewer's font.
func toSVG(title, s string, cols, rows int) string {
	width := float64(cols)*cw + 2*pad
	height := float64(rows)*lh + 2*pad + bar
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="'Cascadia Mono','JetBrains Mono',Consolas,Menlo,monospace" font-size="14">`+"\n", width, height, width, height)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" rx="10" fill="%s" stroke="#1f1f1f"/>`+"\n", bg)
	fmt.Fprintf(&b, `<path d="M10 0 H%.0f a10 10 0 0 1 10 10 V%.0f H0 V10 a10 10 0 0 1 10 -10 Z" fill="#0d0d0d"/><rect y="%.0f" width="100%%" height="1" fill="#1f1f1f"/>`+"\n", width-10, bar, bar)
	for i, c := range []string{"#3a3a3a", "#3a3a3a", "#ff3333"} {
		fmt.Fprintf(&b, `<circle cx="%d" cy="15" r="5" fill="%s"/>`, 18+i*16, c)
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="19.5" fill="#6b6b6b" font-size="12" text-anchor="middle">%s</text>`+"\n", width/2, html.EscapeString(title))
	for row, line := range strings.Split(s, "\n") {
		if row >= rows {
			break
		}
		y := bar + pad + float64(row)*lh + 13
		col := 0
		for _, r := range parse(line) {
			if strings.TrimSpace(r.text) != "" {
				weight := ""
				if r.bold {
					weight = ` font-weight="700"`
				}
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s"%s xml:space="preserve">%s</text>`+"\n",
					pad+float64(col)*cw, y, r.fg, weight, html.EscapeString(r.text))
			}
			col += len([]rune(r.text))
		}
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// parse splits one line into runs by SGR truecolor foreground and bold.
func parse(line string) []run {
	var out []run
	cur := run{fg: fgDefault}
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			out = append(out, run{text.String(), cur.fg, cur.bold})
			text.Reset()
		}
	}
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		if rs[i] == 0x1b && i+1 < len(rs) && rs[i+1] == '[' {
			j := i + 2
			for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
				j++
			}
			if j < len(rs) && rs[j] == 'm' {
				flush()
				apply(&cur, string(rs[i+2:j]))
			}
			i = j
			continue
		}
		text.WriteRune(rs[i])
	}
	flush()
	return out
}

func apply(c *run, params string) {
	p := strings.Split(params, ";")
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case "", "0":
			c.fg, c.bold = fgDefault, false
		case "1":
			c.bold = true
		case "22":
			c.bold = false
		case "39":
			c.fg = fgDefault
		case "38":
			if i+4 < len(p) && p[i+1] == "2" {
				r, _ := strconv.Atoi(p[i+2])
				g, _ := strconv.Atoi(p[i+3])
				bl, _ := strconv.Atoi(p[i+4])
				c.fg = fmt.Sprintf("#%02x%02x%02x", r, g, bl)
				i += 4
			}
		}
	}
}
