package install

import "testing"

func TestPathEntries(t *testing.T) {
	const dir = `C:\Users\me\AppData\Local\Programs\jedkx`
	list := `C:\Windows;%USERPROFILE%\bin;c:\users\me\appdata\local\programs\JEDKX\;D:\tools`

	if !hasEntry(list, dir) {
		t.Fatal("hasEntry must be case- and trailing-slash-insensitive")
	}
	if got := removeEntry(list, dir); got != `C:\Windows;%USERPROFILE%\bin;D:\tools` {
		t.Fatalf("removeEntry kept or dropped the wrong entries: %q", got)
	}
	if got := addEntry(`C:\Windows;`, dir); got != `C:\Windows;`+dir {
		t.Fatalf("addEntry: %q", got)
	}
	if got := addEntry(list, dir); got != list {
		t.Fatal("addEntry must not duplicate")
	}
	if got := addEntry("", dir); got != dir {
		t.Fatalf("addEntry on empty: %q", got)
	}
	if got := removeEntry(`D:\a;D:\jedkx-other`, `D:\jedkx`); got != `D:\a;D:\jedkx-other` {
		t.Fatalf("removeEntry must only drop exact matches: %q", got)
	}
}

func TestIsInside(t *testing.T) {
	cases := []struct {
		p, dir string
		want   bool
	}{
		{"/a/b/c", "/a/b", true},
		{"/a/b", "/a/b", true},
		{"/a/bc", "/a/b", false},
		{"/x", "/a/b", false},
		{"", "/a", false},
	}
	for _, c := range cases {
		if got := isInside(c.p, c.dir); got != c.want {
			t.Errorf("isInside(%q,%q)=%v", c.p, c.dir, got)
		}
	}
}
