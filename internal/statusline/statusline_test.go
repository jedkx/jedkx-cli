package statusline

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderMinimal(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	raw := `{"cwd":"` + dir + `","model":{"display_name":"Claude Opus 5.5"},"context_window":{"used_percentage":42}}`
	out := Render(strings.NewReader(raw))
	plain := ansi.Strip(out)
	for _, want := range []string{filepath.Base(dir), "42%", "opus 5.5"} {
		if !strings.Contains(plain, want) {
			t.Errorf("%q missing in %q", want, plain)
		}
	}
	more := ansi.Strip(Render(strings.NewReader(`{"cwd":"` + dir + `","cost":{"total_duration_ms":754000,"total_lines_added":12,"total_lines_removed":3}}`)))
	for _, want := range []string{"+12 −3", "12m"} {
		if !strings.Contains(more, want) {
			t.Errorf("%q missing in %q", want, more)
		}
	}
	if strings.ContainsAny(plain, "│|⏱") {
		t.Errorf("minimal line must not have separators or timers: %q", plain)
	}
	if !strings.Contains(out, accent) || !strings.Contains(out, green) {
		t.Error("model in red, low context in green")
	}
	if strings.Contains(plain, "▌") {
		t.Error("no bar")
	}
	hot := Render(strings.NewReader(`{"cwd":"` + dir + `","context_window":{"used_percentage":91}}`))
	if !strings.Contains(hot, c(accent, "91%")) {
		t.Error("context above 80% must turn red")
	}
	if Render(strings.NewReader("not json")) == "" {
		t.Error("bad input must still render the folder")
	}
}
