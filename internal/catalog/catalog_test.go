package catalog

import (
	"testing"
	"time"

	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/source"
)

func TestMatchesTurkish(t *testing.T) {
	it := Item{Session: source.Session{Tool: "claude", Title: "İŞLEM ekranı düzeltmesi", Branch: "APP-7321", CWD: `C:\dev\shop-api`}}
	for _, q := range []string{"islem", "ekrani duzelt", "7321", "shop", "claude islem"} {
		if !Matches(it, q) {
			t.Errorf("should match %q", q)
		}
	}
	if Matches(it, "islem codex") {
		t.Error("every word must match")
	}
}

func TestSortPinnedFirst(t *testing.T) {
	now := time.Now()
	items := []Item{
		{Session: source.Session{ID: "old", Updated: now.Add(-time.Hour)}, Entry: meta.Entry{Pinned: true}},
		{Session: source.Session{ID: "new", Updated: now}},
		{Session: source.Session{ID: "mid", Updated: now.Add(-time.Minute)}},
	}
	Sort(items)
	if items[0].ID != "old" || items[1].ID != "new" || items[2].ID != "mid" {
		t.Fatalf("order: %s %s %s", items[0].ID, items[1].ID, items[2].ID)
	}
}
