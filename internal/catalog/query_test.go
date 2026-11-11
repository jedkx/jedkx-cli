package catalog

import (
	"testing"
	"time"

	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/source"
)

func item(title, cwd, branch string, age time.Duration, tags ...string) Item {
	return Item{Session: source.Session{Tool: "claude", ID: "id1", Title: title, CWD: cwd, Branch: branch, Updated: time.Now().Add(-age)},
		Entry: meta.Entry{Tags: tags}}
}

func TestQuery(t *testing.T) {
	a := item("Fix login redirect on İşlem page", `D:\work\belsis-net`, "BELNET-12", time.Hour, "urgent")
	cases := []struct {
		q    string
		want bool
	}{
		{"login", true},
		{"islem", true},            // Turkish folded
		{"redirect login", true},   // any order
		{"logn", true},             // a typo
		{"redirct", true},          // a typo
		{"blsnet", true},           // letters in order inside one word
		{`"login redirect"`, true}, // phrase
		{`"redirect login"`, false},
		{"login -redirect", false},
		{"p:belsis", true},
		{"p:other", false},
		{"b:belnet", true},
		{"#urg", true},
		{"#none", false},
		{"tool:codex", false},
		{"tool:claude login", true},
		{"since:today", true},
		{"older:2d", false},
		{"is:pinned", false},
		{"xyzzy", false},
		{"lo", true},
	}
	for _, c := range cases {
		if got := Matches(a, c.q); got != c.want {
			t.Errorf("%q: got %v want %v", c.q, got, c.want)
		}
	}
}

func TestScoreTitleFirst(t *testing.T) {
	q := Parse("login")
	inTitle, _ := q.Score(item("login page", `C:\x`, "", 0))
	inPath, _ := q.Score(item("something", `C:\login`, "", 0))
	if inTitle <= inPath {
		t.Fatalf("title %d should beat path %d", inTitle, inPath)
	}
}

func TestMatchFuzzyFlag(t *testing.T) {
	q := Parse("logn")
	if _, ok, fz := q.Match(item("login page", `C:\x`, "", 0)); !ok || !fz {
		t.Fatal("logn should match login as a typo")
	}
	if _, ok, fz := Parse("login").Match(item("login page", `C:\x`, "", 0)); !ok || fz {
		t.Fatal("login is an exact match")
	}
}
