package setup

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeHome(t *testing.T) string {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "appdata"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "local"))
	_ = os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	return home
}

// Apply then Undo must leave settings.json exactly as it was (order, other keys).
func TestClaudeRoundTrip(t *testing.T) {
	home := fakeHome(t)
	orig := "{\n  \"model\": \"opus\",\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"node old.js\"\n  },\n  \"permissions\": {\n    \"allow\": [\n      \"Bash(git status)\"\n    ]\n  }\n}\n"
	sp := filepath.Join(home, ".claude", "settings.json")
	_ = os.WriteFile(sp, []byte(orig), 0o600)

	exe := filepath.Join(home, "x y", "jedkx-cli.exe") // a space: the command must be quoted
	if err := Claude(exe); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(sp)
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["theme"] != "custom:jedkx" || !strings.Contains(m["statusLine"].(map[string]any)["command"].(string), `"`+filepath.ToSlash(exe)+`" statusline`) {
		t.Fatalf("not applied: %s", got)
	}
	if strings.Index(string(got), `"model"`) > strings.Index(string(got), `"permissions"`) {
		t.Fatal("key order changed")
	}
	if _, err := os.Stat(themeFile()); err != nil {
		t.Fatal("theme file missing")
	}
	if err := Claude("again.exe"); err != nil { // re-apply must not forget the original
		t.Fatal(err)
	}
	if err := Undo(io.Discard); err != nil {
		t.Fatal(err)
	}
	back, _ := os.ReadFile(sp)
	if string(back) != orig {
		t.Fatalf("undo did not restore:\n%s\nwant:\n%s", back, orig)
	}
	if _, err := os.Stat(themeFile()); !os.IsNotExist(err) {
		t.Fatal("theme file left behind")
	}
}

// A value the user changed after setup is theirs: undo keeps it.
func TestUndoKeepsUserChange(t *testing.T) {
	home := fakeHome(t)
	sp := filepath.Join(home, ".claude", "settings.json")
	if err := Claude("j.exe"); err != nil {
		t.Fatal(err)
	}
	o, _ := readObject(sp)
	o.set("theme", json.RawMessage(`"light"`))
	_ = writeObject(sp, o)
	_ = Undo(io.Discard)
	o, _ = readObject(sp)
	if v, _ := o.get("theme"); string(v) != `"light"` {
		t.Fatalf("user theme lost: %s", v)
	}
	if _, ok := o.get("statusLine"); ok {
		t.Fatal("statusLine did not exist before, must be removed")
	}
}

func TestTerminalFragment(t *testing.T) {
	fakeHome(t)
	if err := Terminal(`C:\x\jedkx-cli.exe`); err != nil {
		t.Skip(err) // non-Windows
	}
	if !ProfileReady() {
		t.Fatal("fragment not written")
	}
	_ = Undo(io.Discard)
	if ProfileReady() {
		t.Fatal("fragment not removed")
	}
}

// Sanity value from Microsoft's fragment docs: app "Git", profile "Git Bash".
func TestFragmentProfileGUID(t *testing.T) {
	if got := FragmentProfileGUID("Git", "Git Bash"); got != "{2ece5bfe-50ed-5f3a-ab87-5cd4baafed2b}" {
		t.Fatalf("got %s", got)
	}
}

func TestDefaultProfileRoundTrip(t *testing.T) {
	home := fakeHome(t)
	wt := filepath.Join(home, "local", "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json")
	_ = os.MkdirAll(filepath.Dir(wt), 0o755)
	orig := "{\n  \"$schema\": \"https://aka.ms/terminal-profiles-schema\",\n  \"defaultProfile\": \"{61c54bbd-c2c6-5271-96e7-009a87ff44bf}\",\n  \"profiles\": {\n    \"list\": []\n  }\n}\n"
	_ = os.WriteFile(wt, []byte(orig), 0o600)
	if err := Terminal(`C:\x\jedkx-cli.exe`); err != nil {
		t.Skip(err)
	}
	o, _ := readObject(wt)
	if v, _ := o.get("defaultProfile"); !strings.Contains(string(v), FragmentProfileGUID("jedkx-cli", ProfileName)) {
		t.Fatalf("the jedkx-cli profile must be the default: %s", v)
	}
	if _, ok := o.getPath("profiles.defaults.colorScheme"); ok {
		t.Fatal("other profiles must keep their own colours")
	}
	if v, _ := o.get("theme"); string(v) != `"jedkx"` {
		t.Fatalf("window theme not set: %s", v)
	}
	_ = Undo(io.Discard)
	if back, _ := os.ReadFile(wt); string(back) != orig {
		t.Fatalf("not restored:\n%s", back)
	}
}

// A real-looking WT file (4-space indent, own scheme and theme) round-trips byte for byte.
func TestWindowsTerminalRoundTrip(t *testing.T) {
	home := fakeHome(t)
	wt := filepath.Join(home, "local", "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json")
	_ = os.MkdirAll(filepath.Dir(wt), 0o755)
	orig := `{
    "$schema": "https://aka.ms/terminal-profiles-schema",
    "defaultProfile": "{61c54bbd-c2c6-5271-96e7-009a87ff44bf}",
    "profiles": {
        "defaults": {
            "colorScheme": "Minimal Dark",
            "cursorShape": "bar"
        },
        "list": []
    },
    "themes": [
        {
            "name": "Minimal Dark"
        }
    ],
    "theme": "Minimal Dark"
}
`
	_ = os.WriteFile(wt, []byte(orig), 0o600)
	if err := Terminal(`C:\x\jedkx-cli.exe`); err != nil {
		t.Skip(err)
	}
	o, _ := readObject(wt)
	for path, want := range map[string]string{"theme": `"jedkx"`, "profiles.defaults.colorScheme": `"Minimal Dark"`} {
		if v, _ := o.getPath(path); string(v) != want {
			t.Fatalf("%s = %s, want %s", path, v, want)
		}
	}
	if v, _ := o.getPath("profiles.defaults.cursorShape"); string(v) != `"bar"` {
		t.Fatal("other profile defaults must stay")
	}
	_ = Undo(io.Discard)
	if back, _ := os.ReadFile(wt); string(back) != orig {
		t.Fatalf("not restored:\n%s", back)
	}
}

// Backup keeps the tree below home: <backup>/.claude/settings.json.
func TestBackupKeepsTree(t *testing.T) {
	home := fakeHome(t)
	_ = os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(`{"a":1}`), 0o600)
	_ = os.MkdirAll(filepath.Join(home, ".claude", "themes"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".claude", "themes", "mine.json"), []byte(`{}`), 0o600)
	root, err := Backup(t.TempDir(), All)
	if err != nil || root == "" {
		t.Fatal(root, err)
	}
	for _, f := range []string{filepath.Join(".claude", "settings.json"), filepath.Join(".claude", "themes", "mine.json")} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Fatalf("%s not backed up", f)
		}
	}
}

// An older setup coloured every profile; applying again gives their colours back.
func TestReapplyRestoresProfileColours(t *testing.T) {
	home := fakeHome(t)
	wt := filepath.Join(home, "local", "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json")
	_ = os.MkdirAll(filepath.Dir(wt), 0o755)
	_ = os.WriteFile(wt, []byte("{\n  \"profiles\": {\n    \"defaults\": {\n      \"colorScheme\": \"jedkx\"\n    }\n  }\n}\n"), 0o600)
	s := state{
		PrevWT: map[string]map[string]json.RawMessage{wt: {"profiles.defaults.colorScheme": json.RawMessage(`"Campbell"`)}},
		OurWT:  map[string]json.RawMessage{"profiles.defaults.colorScheme": json.RawMessage(`"jedkx"`)},
	}
	if err := makeDefault(&s); err != nil {
		t.Skip(err)
	}
	o, _ := readObject(wt)
	if v, _ := o.getPath("profiles.defaults.colorScheme"); string(v) != `"Campbell"` {
		t.Fatalf("old profile colours not given back: %s", v)
	}
}
