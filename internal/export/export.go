// Package export writes a chat as a Markdown file the user can keep or share.
package export

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/source"
)

// Markdown renders the chat: a title, a short facts list, then each message.
func Markdown(it catalog.Item, turns []source.Turn) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", it.Label())
	facts := [][2]string{
		{"tool", it.Tool}, {"project", it.Project()}, {"folder", it.CWD}, {"branch", it.Branch},
		{"last active", it.Updated.Format("2006-01-02 15:04")}, {"session", it.ID},
	}
	for _, f := range facts {
		if f[1] != "" {
			fmt.Fprintf(&b, "- **%s:** %s\n", f[0], f[1])
		}
	}
	b.WriteString("\n---\n")
	ai := map[string]string{"claude": "Claude", "codex": "Codex"}[it.Tool]
	if ai == "" {
		ai = "Assistant"
	}
	for _, t := range turns {
		who := ai
		if t.User {
			who = "You"
		}
		fmt.Fprintf(&b, "\n### %s\n\n%s\n", who, t.Text)
	}
	return b.String()
}

// Save exports it into the Downloads folder and returns the file path.
func Save(it catalog.Item) (string, error) {
	a := catalog.Adapter(it.Tool)
	tr, ok := a.(source.Transcriber)
	if a == nil || !ok {
		return "", errors.New("export is not supported for " + it.Tool)
	}
	turns, err := tr.Transcript(it.Session)
	if err != nil {
		return "", err
	}
	if len(turns) == 0 {
		return "", errors.New("nothing to export: the chat has no text messages")
	}
	id := it.ID
	if len(id) > 8 {
		id = id[:8]
	}
	// the id keeps two chats with the same project, minute and title apart
	name := fmt.Sprintf("%s-%s-%s-%s.md", slug(it.Project(), 24), it.Updated.Format("20060102-1504"), slug(it.Label(), 40), slug(id, 8))
	p := filepath.Join(paths.Downloads(), name)
	return p, os.WriteFile(p, []byte(Markdown(it, turns)), 0o644)
}

// slug keeps letters and digits, joins the rest with '-', at most n runes.
func slug(s string, n int) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
		if len([]rune(b.String())) >= n {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "chat"
	}
	return out
}
