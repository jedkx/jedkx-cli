package install

import (
	"path/filepath"
	"strings"
)

// Pure PATH-list helpers (tested). Windows semantics: ';' separated, case-insensitive,
// trailing separators ignored. Only the exact entry we own is ever removed.

func normEntry(e string) string {
	e = strings.TrimSpace(strings.Trim(e, `"`))
	e = strings.TrimRight(e, `\/`)
	return strings.ToLower(filepath.Clean(e))
}

func hasEntry(list, dir string) bool {
	want := normEntry(dir)
	for _, e := range strings.Split(list, ";") {
		if e != "" && normEntry(e) == want {
			return true
		}
	}
	return false
}

func addEntry(list, dir string) string {
	if hasEntry(list, dir) {
		return list
	}
	list = strings.TrimRight(list, ";")
	if list == "" {
		return dir
	}
	return list + ";" + dir
}

func removeEntry(list, dir string) string {
	want := normEntry(dir)
	parts := strings.Split(list, ";")
	keep := parts[:0]
	for _, e := range parts {
		if e != "" && normEntry(e) == want {
			continue
		}
		keep = append(keep, e)
	}
	return strings.Join(keep, ";")
}
