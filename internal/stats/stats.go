// Package stats sums token use per day from the chats' own logs.
package stats

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/jedkx/jedkx-cli/internal/catalog"
	"github.com/jedkx/jedkx-cli/internal/source"
)

// Day is one row of the report.
type Day struct {
	Date                         string
	Chats, Replies               int
	In, Out, CacheRead, CacheNew int
	Models                       map[string]int // replies per model
	chats                        map[string]bool
}

// Collect sums usage of the last `days` days (local time) over items.
func Collect(items []catalog.Item, days int) []Day {
	now := time.Now()
	since := time.Date(now.Year(), now.Month(), now.Day()-days+1, 0, 0, 0, 0, time.Local) // local midnight
	byDate := map[string]*Day{}
	for _, it := range items {
		if it.Updated.Before(since) {
			continue // not active in the window: skip reading the file
		}
		ur, ok := catalog.Adapter(it.Tool).(source.UsageReader)
		if !ok {
			continue
		}
		chatDays := map[string]bool{}
		for _, u := range ur.Usage(it.Session) {
			t := u.Time.Local()
			if t.Before(since) {
				continue
			}
			k := t.Format("2006-01-02")
			d := byDate[k]
			if d == nil {
				d = &Day{Date: k, Models: map[string]int{}, chats: map[string]bool{}}
				byDate[k] = d
			}
			if !chatDays[k] {
				chatDays[k] = true
				d.Chats++
				d.chats[it.Key()] = true
			}
			d.Replies++
			d.In += u.In
			d.Out += u.Out
			d.CacheRead += u.CacheRead
			d.CacheNew += u.CacheNew
			if u.Model != "" && !strings.HasPrefix(u.Model, "<") { // "<synthetic>" = not a real model call
				d.Models[u.Model]++
			}
		}
	}
	out := make([]Day, 0, len(byDate))
	for _, d := range byDate {
		out = append(out, *d)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Date < out[b].Date })
	return out
}

// Print writes the report as an aligned table with a total line.
func Print(w io.Writer, rows []Day) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "  no token use recorded in this period")
		return
	}
	fmt.Fprintf(w, "  %-10s %6s %8s %9s %9s %11s %11s\n", "day", "chats", "replies", "input", "output", "cache read", "cache new")
	var t Day
	models := map[string]int{}
	distinct := map[string]bool{} // a chat active on 3 days is one chat in the total
	for _, d := range rows {
		fmt.Fprintf(w, "  %-10s %6d %8d %9s %9s %11s %11s\n", d.Date, d.Chats, d.Replies, short(d.In), short(d.Out), short(d.CacheRead), short(d.CacheNew))
		for k := range d.chats {
			distinct[k] = true
		}
		t.Replies += d.Replies
		t.In += d.In
		t.Out += d.Out
		t.CacheRead += d.CacheRead
		t.CacheNew += d.CacheNew
		for m, n := range d.Models {
			models[m] += n
		}
	}
	t.Chats = len(distinct)
	fmt.Fprintf(w, "  %-10s %6d %8d %9s %9s %11s %11s\n", "total", t.Chats, t.Replies, short(t.In), short(t.Out), short(t.CacheRead), short(t.CacheNew))
	var ms []string
	for m, n := range models {
		ms = append(ms, fmt.Sprintf("%s %d", m, n))
	}
	sort.Strings(ms)
	fmt.Fprintln(w, "\n  models  "+strings.Join(ms, " · "))
	fmt.Fprintln(w, "  tokens only — prices change, so no cost is guessed. Claude Code logs only; Codex has no usage here.")
}

// short is 1234 → 1.2k, 1234567 → 1.2M.
func short(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}
