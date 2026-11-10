package export

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/source"
)

func TestMarkdown(t *testing.T) {
	it := catalog.Item{Session: source.Session{Tool: "claude", ID: "abc", Title: "Fix login", CWD: filepath.Join("work", "app")}}
	md := Markdown(it, []source.Turn{{User: true, Text: "why?"}, {Text: "because"}})
	for _, want := range []string{"# Fix login", "- **project:** app", "### You\n\nwhy?", "### Claude\n\nbecause"} {
		if !strings.Contains(md, want) {
			t.Fatalf("%q missing:\n%s", want, md)
		}
	}
	if got := slug("Çalışma: test / prod!", 40); got != "çalışma-test-prod" {
		t.Fatalf("slug %q", got)
	}
}
