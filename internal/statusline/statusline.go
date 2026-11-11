// Package statusline renders the Claude Code status line. Claude runs
// `jedkx-cli statusline` with session JSON on stdin (settings.json statusLine).
//
// Minimal, like the portfolio: folder, branch (±dirty ↑↓), Jira key, context %,
// model — grey type, spaces instead of separators; context goes green → yellow →
// red as it fills; the model and the Jira key in the jedkx red.
package statusline

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jedkx/jedkx-cli/internal/account"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type input struct {
	CWD       string `json:"cwd"`
	Workspace struct {
		ProjectDir string `json:"project_dir"`
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Context *struct {
		Used *float64 `json:"used_percentage"`
	} `json:"context_window"`
	Cost struct {
		DurationMS float64 `json:"total_duration_ms"`
		Added      int     `json:"total_lines_added"`
		Removed    int     `json:"total_lines_removed"`
	} `json:"cost"`
	Model struct {
		Name string `json:"display_name"`
		ID   string `json:"id"`
	} `json:"model"`
}

// Palette — J-Portfolio (same values as internal/ui/theme.go).
const (
	text   = "38;2;245;245;245"
	muted  = "38;2;154;154;154"
	dim    = "38;2;94;94;94"
	accent = "38;2;255;51;51"
	green  = "38;2;0;255;0" // terminal green
	faint  = "38;2;42;42;42"
	blue   = "38;2;90;160;255"
	yellow = "38;2;255;170;0"
)

func c(code, s string) string { return "\x1b[" + code + "m" + s + "\x1b[0m" }

var jiraKey = regexp.MustCompile(`[A-Z][A-Z0-9]+-\d+`)

// Render reads Claude's JSON from r and returns the status line. Never fails:
// any missing piece is simply left out.
func Render(r io.Reader) string {
	var d input
	raw, _ := io.ReadAll(r)
	_ = json.Unmarshal(raw, &d)
	var parts []string

	dir := firstNonEmpty(d.Workspace.ProjectDir, d.Workspace.CurrentDir, d.CWD)
	if dir == "" {
		dir, _ = os.Getwd()
	}
	used := -1.0
	if d.Context != nil && d.Context.Used != nil {
		used = *d.Context.Used
	}
	parts = append(parts, c(text+";1", filepath.Base(dir)))

	br, dirty, ahead, behind := repo(dir)
	if br != "" {
		s := c(muted, br)
		var extra []string
		if dirty > 0 {
			extra = append(extra, fmt.Sprintf("±%d", dirty))
		}
		if ahead > 0 {
			extra = append(extra, fmt.Sprintf("↑%d", ahead))
		}
		if behind > 0 {
			extra = append(extra, fmt.Sprintf("↓%d", behind))
		}
		if len(extra) > 0 {
			s += " " + c(dim, strings.Join(extra, " "))
		}
		parts = append(parts, s)
		if k := jiraKey.FindString(br); k != "" {
			parts = append(parts, c(accent, k))
		}
	}

	if used >= 0 {
		parts = append(parts, meter(used)+" "+c(heat(used), fmt.Sprintf("%.0f%%", used)))
	}
	// what this chat changed: lines added (blue) / removed (red), like the diffs
	if d.Cost.Added > 0 || d.Cost.Removed > 0 {
		parts = append(parts, c(blue, fmt.Sprintf("+%d", d.Cost.Added))+" "+c(accent, fmt.Sprintf("−%d", d.Cost.Removed)))
	}
	if t := since(d.Cost.DurationMS); t != "" {
		parts = append(parts, c(dim, t))
	}
	if n := firstNonEmpty(d.Model.Name, d.Model.ID); n != "" {
		parts = append(parts, c(accent+";1", strings.ToLower(strings.TrimPrefix(n, "Claude "))))
	}
	if e := account.Claude(); e != "" { // which account this chat runs on, quietly
		parts = append(parts, c(dim, e))
	}
	return strings.Join(parts, "   ")
}

// heat: the fuller the context, the warmer the colour.
func heat(u float64) string {
	switch {
	case u > 80:
		return accent
	case u > 50:
		return yellow
	}
	return green
}

// meter is a 10-cell context bar; filled cells warm up along the bar
// (green, then yellow, then red), empty cells are faint.
func meter(u float64) string {
	const cells = 10
	n := int(u/10 + 0.5)
	var b strings.Builder
	for i := 0; i < cells; i++ {
		if i >= n {
			b.WriteString(c(faint, "━"))
			continue
		}
		b.WriteString(c(heat(float64(i*10+5)), "━"))
	}
	return b.String()
}

// since is the chat's age, short: 45s, 12m, 1h05.
func since(ms float64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return ""
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%02d", int(d.Hours()), int(d.Minutes())%60)
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// repo asks git once for branch, changed files and ahead/behind; Claude runs
// the status line on every message, so it is one call with one time limit.
func repo(dir string) (branch string, dirty, ahead, behind int) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "status", "--porcelain=v2", "--branch")
	cmd.Dir = dir
	cmd.WaitDelay = 200 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return "", 0, 0, 0
	}
	var oid string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		switch {
		case strings.HasPrefix(l, "# branch.oid "):
			oid = strings.TrimPrefix(l, "# branch.oid ")
		case strings.HasPrefix(l, "# branch.head "):
			branch = strings.TrimPrefix(l, "# branch.head ")
		case strings.HasPrefix(l, "# branch.ab "):
			_, _ = fmt.Sscanf(strings.TrimPrefix(l, "# branch.ab "), "+%d -%d", &ahead, &behind)
		case l != "" && !strings.HasPrefix(l, "#"):
			dirty++
		}
	}
	if branch == "(detached)" && len(oid) >= 7 {
		branch = oid[:7]
	}
	return branch, dirty, ahead, behind
}
