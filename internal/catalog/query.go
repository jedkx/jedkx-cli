package catalog

import (
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Query is a parsed search. Plain words must all be found (in the title, first
// prompt, project, folder, branch, tags or id), with small typos forgiven;
// "quoted words" must appear as written; -word must not appear. Filters:
//
//	tool:claude  tool:codex    p:<project>   b:<branch>   #tag
//	since:3d  since:2w  since:today   older:30d   is:pinned
type Query struct {
	Words, Phrases, Not []string
	Tool, Project       string
	Branch, Tag         string
	Since, Older        time.Time
	Pinned              bool
}

// Empty reports whether the query asks for nothing.
func (q Query) Empty() bool {
	return len(q.Words)+len(q.Phrases)+len(q.Not) == 0 && q.Tool == "" && q.Project == "" &&
		q.Branch == "" && q.Tag == "" && q.Since.IsZero() && q.Older.IsZero() && !q.Pinned
}

// Terms are the words and phrases that look at text (for the inside-chat search).
func (q Query) Terms() []string { return append(append([]string{}, q.Words...), q.Phrases...) }

// Parse reads a search line.
func Parse(s string) Query {
	var q Query
	now := time.Now()
	for _, tok := range splitQuoted(s) {
		if tok.quoted {
			if t := Fold(strings.TrimSpace(tok.text)); t != "" {
				q.Phrases = append(q.Phrases, t)
			}
			continue
		}
		t := Fold(tok.text)
		key, val, has := strings.Cut(t, ":")
		switch {
		case strings.HasPrefix(t, "-") && len(t) > 1:
			q.Not = append(q.Not, t[1:])
		case strings.HasPrefix(t, "#") && len(t) > 1:
			q.Tag = t[1:]
		case has && val != "" && (key == "tool" || key == "t"):
			q.Tool = val
		case has && val != "" && (key == "p" || key == "proj" || key == "project"):
			q.Project = val
		case has && val != "" && (key == "b" || key == "branch"):
			q.Branch = val
		case has && key == "since":
			if d, ok := ago(val, now); ok {
				q.Since = d
			}
		case has && (key == "older" || key == "before"):
			if d, ok := ago(val, now); ok {
				q.Older = d
			}
		case t == "is:pinned" || t == "is:pin":
			q.Pinned = true
		default:
			q.Words = append(q.Words, t)
		}
	}
	return q
}

type token struct {
	text   string
	quoted bool
}

func splitQuoted(s string) []token {
	var out []token
	for len(s) > 0 {
		s = strings.TrimLeftFunc(s, unicode.IsSpace)
		if s == "" {
			break
		}
		if s[0] == '"' {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 { // still typing the phrase
				out = append(out, token{s[1:], true})
				break
			}
			out = append(out, token{s[1 : end+1], true})
			s = s[end+2:]
			continue
		}
		end := strings.IndexFunc(s, unicode.IsSpace)
		if end < 0 {
			end = len(s)
		}
		out = append(out, token{s[:end], false})
		s = s[end:]
	}
	return out
}

// ago turns "3d", "2w", "5h", "today", "yesterday" into a moment.
func ago(v string, now time.Time) (time.Time, bool) {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch v {
	case "today", "bugun":
		return midnight, true
	case "yesterday", "dun":
		return midnight.AddDate(0, 0, -1), true
	}
	if len(v) < 2 {
		return time.Time{}, false
	}
	n, err := strconv.Atoi(v[:len(v)-1])
	if err != nil || n < 0 {
		return time.Time{}, false
	}
	switch v[len(v)-1] {
	case 'h', 's': // s: saat
		return now.Add(-time.Duration(n) * time.Hour), true
	case 'd', 'g': // g: gün
		return now.AddDate(0, 0, -n), true
	case 'w':
		return now.AddDate(0, 0, -7*n), true
	case 'm', 'a': // a: ay
		return now.AddDate(0, -n, 0), true
	}
	return time.Time{}, false
}

// Filter applies the filters (tool, project, branch, tag, dates, pinned).
func (q Query) Filter(i Item) bool {
	if q.Tool != "" && !strings.HasPrefix(i.Tool, q.Tool) {
		return false
	}
	if q.Project != "" && !strings.Contains(Fold(i.Project()+" "+i.CWD), q.Project) {
		return false
	}
	if q.Branch != "" && !strings.Contains(Fold(i.Branch), q.Branch) {
		return false
	}
	if q.Tag != "" {
		found := false
		for _, t := range i.Tags {
			if strings.HasPrefix(Fold(t), q.Tag) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if !q.Since.IsZero() && i.Updated.Before(q.Since) {
		return false
	}
	if !q.Older.IsZero() && i.Updated.After(q.Older) {
		return false
	}
	return !q.Pinned || i.Pinned
}

// field weights: a hit in the title counts most.
type field struct {
	text   string
	weight int
}

func fields(i Item) []field {
	return []field{
		{Fold(i.Label()), 10},
		{Fold(strings.Join(i.Tags, " ")), 8},
		{Fold(i.Project()), 6},
		{Fold(i.Prompt), 4},
		{Fold(i.Branch), 3},
		{Fold(i.CWD), 2},
		{Fold(i.Tool + " " + i.ID), 1},
	}
}

// Score tells whether the text part of q matches i, and how well (higher is
// better). Filters are not checked here: see Filter.
func (q Query) Score(i Item) (int, bool) {
	s, ok, _ := q.Match(i)
	return s, ok
}

// Match is Score, and also whether a word only matched with a typo.
func (q Query) Match(i Item) (score int, ok, fuzzy bool) {
	fs := fields(i)
	for _, n := range q.Not {
		for _, f := range fs {
			if strings.Contains(f.text, n) {
				return 0, false, false
			}
		}
	}
	score = 0
	for _, p := range q.Phrases {
		best := 0
		for _, f := range fs {
			if strings.Contains(f.text, p) {
				best = max(best, 3*f.weight)
			}
		}
		if best == 0 {
			return 0, false, false
		}
		score += best
	}
	for _, w := range q.Words {
		best, exact := 0, false
		for _, f := range fs {
			ws := wordScore(f.text, w)
			best = max(best, ws*f.weight)
			exact = exact || ws > 1
		}
		if best == 0 {
			return 0, false, false
		}
		fuzzy = fuzzy || !exact
		score += best
	}
	return score, true, fuzzy
}

// wordScore: 4 a whole word, 3 a word start, 2 inside a word, 1 a near miss
// (one or two typos, or the letters in order inside one word: "blsnet"); 0 none.
func wordScore(text, w string) int {
	if i := strings.Index(text, w); i >= 0 {
		start := i == 0 || !isWordRune(rune(text[i-1]))
		end := i+len(w) == len(text) || !isWordRune(rune(text[i+len(w)]))
		switch {
		case start && end:
			return 4
		case start:
			return 3
		}
		return 2
	}
	if len([]rune(w)) < 4 {
		return 0
	}
	allowed := 1
	if len([]rune(w)) >= 8 {
		allowed = 2
	}
	for _, tok := range strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) || r == '/' || r == '\\' }) {
		if near(tok, w, allowed) || inOrder(tok, w) {
			return 1
		}
		for _, part := range strings.FieldsFunc(tok, func(r rune) bool { return !isWordRune(r) }) {
			if near(part, w, allowed) {
				return 1
			}
		}
	}
	return 0
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// inOrder: every letter of w, in order, inside tok, starting at its first letter.
func inOrder(tok, w string) bool {
	t, p := []rune(tok), []rune(w)
	if len(t) == 0 || len(p) == 0 || t[0] != p[0] || len(t) > 3*len(p) {
		return false
	}
	j := 0
	for _, r := range t {
		if j < len(p) && r == p[j] {
			j++
		}
	}
	return j == len(p)
}

// near: a and b differ by at most k edits (insert, delete, change, swap two).
// It also lets w match the start of a longer word with a typo ("belsys" → "belsisnet").
func near(a, b string, k int) bool {
	x, y := []rune(a), []rune(b)
	if len(x) > len(y)+k { // compare b with a's start of the same length
		x = x[:len(y)+k]
	}
	if abs(len(x)-len(y)) > k {
		return false
	}
	prev2 := make([]int, len(y)+1)
	prev := make([]int, len(y)+1)
	cur := make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	best := len(y)
	for i := 1; i <= len(x); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
			rowMin = min(rowMin, cur[j])
		}
		if rowMin > k {
			return false
		}
		if i >= len(y)-k {
			best = min(best, cur[len(y)])
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return best <= k
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
