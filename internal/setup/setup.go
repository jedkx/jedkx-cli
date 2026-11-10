// Package setup dresses Claude Code and Windows Terminal in the jedkx look — only
// when the user asks (install offers it, `jedkx-cli setup` runs it) — and puts
// everything back on `setup --undo` / uninstall.
//
//   - Claude Code: statusLine → jedkx-cli statusline, theme → custom:jedkx
//     (~/.claude/themes/jedkx.json). Previous values are saved and restored.
//   - Windows Terminal: a "jedkx" colour scheme + "jedkx-cli" profile through a
//     settings fragment (only that profile uses the jedkx colours); in WT's
//     settings.json only the window theme (grey tabs) changes. Old values are
//     saved and restored.
package setup

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"unicode/utf16"

	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/paths"
)

// Palette — J-Portfolio (same values as internal/ui/theme.go).
const (
	void   = "#020202"
	text   = "#f5f5f5"
	muted  = "#9a9a9a"
	dim    = "#5e5e5e"
	faint  = "#2a2a2a"
	accent = "#ff3333"
)

// ProfileName is the Windows Terminal profile jedkx-cli adds.
const ProfileName = "jedkx-cli"

func claudeDir() string      { return filepath.Join(paths.Home(), ".claude") }
func claudeSettings() string { return filepath.Join(claudeDir(), "settings.json") }
func themeFile() string      { return filepath.Join(claudeDir(), "themes", "jedkx.json") }
func stateFile() string      { return filepath.Join(paths.ConfigDir(), "setup.json") }

// FragmentDir is where Windows Terminal picks up third-party settings fragments.
func FragmentDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Microsoft", "Windows Terminal", "Fragments", "jedkx-cli")
}

// state remembers what Claude Code had before, so undo restores it exactly.
type state struct {
	Claude bool `json:"claude"`
	// Claude Code settings keys before / after setup ("null" = key did not exist).
	PrevClaude map[string]json.RawMessage `json:"prev_claude,omitempty"`
	OurClaude  map[string]json.RawMessage `json:"our_claude,omitempty"`
	Terminal   bool                       `json:"terminal"`
	// Windows Terminal keys per settings file before setup ("null" = unset), and ours.
	PrevWT map[string]map[string]json.RawMessage `json:"prev_wt,omitempty"`
	OurWT  map[string]json.RawMessage            `json:"our_wt,omitempty"`
}

func loadState() state {
	var s state
	if b, err := os.ReadFile(stateFile()); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

func saveState(s state) error {
	if !s.Claude && !s.Terminal {
		err := os.Remove(stateFile())
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return config.WriteAtomic(stateFile(), b)
}

// ErrNoTerminal: no terminal jedkx-cli can dress here (only Windows Terminal is).
var ErrNoTerminal = errors.New("only Windows Terminal is configured automatically; other terminals keep their own colours")

// Applied reports what setup has changed on this machine.
func Applied() (claude, terminal bool) {
	s := loadState()
	return s.Claude, s.Terminal
}

// Parts are the pieces of the jedkx look the user picked.
type Parts struct{ Claude, Terminal bool }

// All is every part.
var All = Parts{Claude: true, Terminal: true}

// Apply runs the chosen parts; exe is the jedkx-cli binary that Claude and WT should call.
func Apply(exe string, p Parts, out io.Writer) error {
	var errC, errT error
	if p.Claude {
		errC = Claude(exe)
		report(out, "Claude Code", "theme, status line, spinner words, startup line  ("+claudeSettings()+")", errC)
	}
	if !p.Terminal {
		return errC
	}
	errT = Terminal(exe)
	if errors.Is(errT, ErrNoTerminal) {
		fmt.Fprintf(out, "  · %-12s skipped: %v\n", "Terminal", errT)
		errT = nil
	} else {
		report(out, "Terminal", "jedkx-cli profile (default, jedkx colours), grey tabs  (Windows Terminal)", errT)
	}
	return errors.Join(errC, errT)
}

func report(out io.Writer, what, detail string, err error) {
	if err != nil {
		fmt.Fprintf(out, "  ! %-12s %v\n", what, err)
		return
	}
	fmt.Fprintf(out, "  ✓ %-12s %s\n", what, detail)
}

// Undo restores everything Apply changed. Safe to run when nothing was applied.
func Undo(out io.Writer) error { return UndoOnly(out, All) }

// UndoOnly puts back only the parts in p (setup --undo --only terminal).
func UndoOnly(out io.Writer, p Parts) error {
	s := loadState()
	var errs []error
	if s.Terminal && p.Terminal {
		if err := undoDefault(s); err != nil {
			// keep the scheme and the saved values: WT still points at "jedkx"
			errs = append(errs, err)
		} else if d := FragmentDir(); d != "" {
			if err := os.RemoveAll(d); err != nil {
				errs = append(errs, err)
			} else {
				s.Terminal = false
				fmt.Fprintln(out, "  ✓ Terminal     jedkx-cli profile removed, your default profile and theme back")
			}
		}
	}
	if s.Claude && p.Claude {
		if err := undoClaude(s); err != nil {
			errs = append(errs, err)
		} else {
			s.Claude = false
			fmt.Fprintln(out, "  ✓ Claude Code  previous status line and theme restored")
		}
	}
	errs = append(errs, saveState(s))
	return errors.Join(errs...)
}

// ---------------------------------------------------------------- Claude Code

// claudeTheme is J-Portfolio in Claude Code's tokens (code.claude.com/docs ›
// terminal config › custom themes): white text on black, one red accent, greys
// for everything quiet. Unknown tokens are ignored by Claude Code, so this list
// can grow safely.
var claudeTheme = map[string]any{
	"name": "jedkx",
	"base": "dark-daltonized", // blue/red diffs
	"overrides": map[string]string{
		// text and accent
		"claude": accent, "claudeShimmer": "#ff7a7a",
		"text": text, "inverseText": void,
		"inactive": "#6b6b6b", "inactiveShimmer": muted,
		"subtle": faint, "suggestion": text, "remember": "#bdbdbd",
		"permission": accent, "permissionShimmer": "#ff7a7a",
		// status
		"success": "#00ff00", "error": accent, "warning": "#ffaa00", "warningShimmer": "#ffc94d", "merged": "#bdbdbd",
		// input and modes
		"promptBorder": faint, "promptBorderShimmer": dim,
		"planMode": text, "autoAccept": accent, "bashBorder": dim, "ide": muted,
		"fastMode": accent, "fastModeShimmer": "#ff7a7a",
		// diffs keep Claude's own dark colours (they read best)
		// transcript backgrounds
		// your messages sit on a faint red-black, like the selected row in the list
		"userMessageBackground": "#170909", "userMessageBackgroundHover": "#220c0c",
		"bashMessageBackgroundColor": "#0b0b0b", "memoryBackgroundColor": "#0b0b0b",
		"selectionBg": "#3a0d0d", "composerSidebarBackground": "#0b0b0b",
		// labels and meters
		"briefLabelYou": text, "briefLabelClaude": accent,
		"rate_limit_fill": accent, "rate_limit_empty": faint,
	},
}

// spinnerTips replace Claude's tips with jedkx-cli ones while it works.
var spinnerTips = []string{
	"jedkx-cli: enter opens a chat beside the list",
	"jedkx-cli: alt+shift+← → makes the list wider or narrower",
	"jedkx-cli: / searches every Claude and Codex chat",
	"jedkx-cli: ← closes a project group, z closes them all",
	"jedkx-cli: p pins a chat to the top",
	"jedkx-cli: closing the list closes the chats it opened",
}

// spinner words shown while Claude works — the portfolio's world (Nox, the
// receiver, the decrypt game) instead of the default verbs.
var spinnerVerbs = []string{
	"Nox is thinking", "Nox is chewing on it", "Decrypting", "Tuning the signal",
	"Following the signal", "Reading the fragments", "Scanning", "Routing",
}

// claudeKeys are the Claude Code settings jedkx-cli sets, for the given binary.
func claudeKeys(exe string) map[string]any {
	return map[string]any{
		// by absolute path, so it works in sessions started before PATH changed
		"statusLine":           map[string]string{"type": "command", "command": quote(exe) + " statusline"},
		"theme":                "custom:jedkx",
		"spinnerVerbs":         map[string]any{"mode": "replace", "verbs": spinnerVerbs},
		"companyAnnouncements": []string{"jedkx-cli · Nox is watching your sessions"},
		"spinnerTipsOverride":  map[string]any{"label": "jedkx-cli", "tips": spinnerTips, "excludeDefault": true},
		// no OSC 9;4 progress: Windows Terminal would colour the tab, over the grey jedkx tabs
		"terminalProgressBarEnabled": false,
	}
}

// Claude dresses Claude Code in the jedkx look: theme, status line, spinner words
// and a startup line. The user's previous values are saved once and restored by Undo.
func Claude(exe string) error {
	if _, err := os.Stat(claudeDir()); err != nil {
		return errors.New("Claude Code is not set up on this machine (~/.claude missing)")
	}
	obj, err := readObject(claudeSettings()) // read first: nothing is written if it cannot be parsed
	if err != nil {
		return err
	}
	tb, _ := json.MarshalIndent(claudeTheme, "", "  ")
	if err := os.MkdirAll(filepath.Dir(themeFile()), 0o755); err != nil {
		return err
	}
	if err := config.WriteAtomic(themeFile(), append(tb, '\n')); err != nil {
		return err
	}
	s := loadState()
	if s.PrevClaude == nil {
		s.PrevClaude = map[string]json.RawMessage{}
	}
	oldOurs := s.OurClaude
	s.OurClaude = map[string]json.RawMessage{}
	for k, v := range claudeKeys(exe) {
		cur, ok := obj.get(k)
		if !ok {
			cur = json.RawMessage("null")
		}
		// remember the user's value: the first time, and again whenever they
		// changed it since our last run (so undo gives back their latest choice)
		if _, seen := s.PrevClaude[k]; !seen || !sameJSON(cur, oldOurs[k]) {
			s.PrevClaude[k] = cur
		}
		ours, _ := json.Marshal(v)
		obj.set(k, ours)
		s.OurClaude[k] = ours
	}
	if err := writeObject(claudeSettings(), obj); err != nil {
		return err
	}
	s.Claude = true
	return saveState(s)
}

func undoClaude(s state) error {
	obj, err := readObject(claudeSettings())
	if err != nil {
		return err
	}
	for k, prev := range s.PrevClaude {
		if cur, ok := obj.get(k); ok && !sameJSON(cur, s.OurClaude[k]) {
			continue // the user changed it since; leave their choice alone
		}
		if string(prev) == "null" {
			obj.del(k)
		} else {
			obj.set(k, prev)
		}
	}
	if _, err := os.Stat(claudeDir()); err != nil {
		return nil // Claude Code was removed meanwhile: do not recreate its folder
	}
	if err := writeObject(claudeSettings(), obj); err != nil {
		return err
	}
	if err := os.Remove(themeFile()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func quote(p string) string {
	p = filepath.ToSlash(p)
	if bytes.ContainsAny([]byte(p), " \t") {
		return `"` + p + `"`
	}
	return p
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xb, yb)
}

// ---------------------------------------------------------------- Windows Terminal

// Terminal installs the jedkx scheme and profile as a WT settings fragment.
func Terminal(_ string) error {
	if runtime.GOOS != "windows" {
		return ErrNoTerminal
	}
	dir := FragmentDir()
	if dir == "" {
		return errors.New("LOCALAPPDATA is not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := config.WriteAtomic(filepath.Join(dir, "jedkx-cli.json"), fragment()); err != nil {
		return err
	}
	s := loadState()
	s.Terminal = true
	err := makeDefault(&s)
	return errors.Join(err, saveState(s))
}

// RefreshTerminal rewrites the fragment when this version's profile differs
// from the one on disk (after an update). Only if the terminal part is applied.
func RefreshTerminal() {
	s := loadState()
	if runtime.GOOS != "windows" || !s.Terminal || !ProfileReady() {
		return
	}
	for k := range wtKeys() { // a new version sets a key the old one did not
		if _, ok := s.OurWT[k]; !ok {
			if makeDefault(&s) == nil {
				_ = saveState(s)
			}
			break
		}
	}
	f := filepath.Join(FragmentDir(), "jedkx-cli.json")
	if old, err := os.ReadFile(f); err == nil && string(old) == string(fragment()) {
		return
	}
	_ = config.WriteAtomic(f, fragment())
}

// fragment is the WT settings fragment: the jedkx scheme and the jedkx-cli profile.
func fragment() []byte {
	frag := map[string]any{
		"schemes": []map[string]string{{
			"name": "jedkx", "background": void, "foreground": text,
			"cursorColor": accent, "selectionBackground": "#3a0d0d",
			"black": void, "brightBlack": dim,
			"red": accent, "brightRed": "#ff6b6b",
			"green": "#2ecc71", "brightGreen": "#5fe39a",
			"yellow": "#ffaa00", "brightYellow": "#ffc94d",
			"blue": muted, "brightBlue": "#c8c8c8",
			"purple": "#bdbdbd", "brightPurple": "#e0e0e0",
			"cyan": "#bdbdbd", "brightCyan": "#e0e0e0",
			"white": "#d6d6d6", "brightWhite": text,
		}},
		"profiles": []map[string]any{{
			// a normal shell in the jedkx colours; type jedkx-cli to open the list
			"name":              ProfileName,
			"commandline":       shell(),
			"startingDirectory": "%USERPROFILE%",
			"colorScheme":       "jedkx",
			"font":              map[string]any{"face": "Cascadia Mono", "size": 11},
			"padding":           "14, 10",
			"cursorShape":       "bar",
			"scrollbarState":    "hidden",
			"closeOnExit":       "always", // panes opened beside the list close with their chat
		}},
	}
	b, _ := json.MarshalIndent(frag, "", "  ")
	return append(b, '\n')
}

// shell is what the jedkx-cli profile runs: PowerShell 7 when installed,
// else Windows PowerShell.
func shell() string {
	if p, err := exec.LookPath("pwsh.exe"); err == nil {
		return quote(p) + " -NoLogo"
	}
	return "powershell.exe -NoLogo"
}

// ProfileReady reports whether new tabs/panes can use the jedkx-cli profile.
func ProfileReady() bool {
	d := FragmentDir()
	if d == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(d, "jedkx-cli.json"))
	return err == nil
}

// ---------------------------------------------------------------- default profile

// fragmentNS is Windows Terminal's namespace for profiles that come from fragments.
var fragmentNS = [16]byte{0xf6, 0x5d, 0xdb, 0x7e, 0x70, 0x6b, 0x44, 0x99, 0x8a, 0x50, 0x40, 0x31, 0x3c, 0xaf, 0x51, 0x0a}

// FragmentProfileGUID is the GUID Windows Terminal gives a fragment profile:
// v5(v5(fragmentNS, app), profile), names hashed as UTF-16LE.
func FragmentProfileGUID(app, profile string) string {
	ns := uuid5(fragmentNS, utf16le(app))
	g := uuid5(ns, utf16le(profile))
	return fmt.Sprintf("{%x-%x-%x-%x-%x}", g[0:4], g[4:6], g[6:8], g[8:10], g[10:16])
}

func utf16le(s string) []byte {
	var b []byte
	for _, r := range utf16.Encode([]rune(s)) {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}

func uuid5(ns [16]byte, name []byte) [16]byte {
	h := sha1.New()
	h.Write(ns[:])
	h.Write(name)
	var u [16]byte
	copy(u[:], h.Sum(nil))
	u[6] = u[6]&0x0f | 0x50
	u[8] = u[8]&0x3f | 0x80
	return u
}

// wtSettingsFiles are Windows Terminal's settings.json locations (store, preview,
// unpackaged) that exist on this machine.
func wtSettingsFiles() []string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return nil
	}
	candidates := []string{
		filepath.Join(base, "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json"),
		filepath.Join(base, "Packages", "Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe", "LocalState", "settings.json"),
		filepath.Join(base, "Microsoft", "Windows Terminal", "settings.json"),
	}
	var out []string
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// wtTheme is the window chrome (tab row, tabs) in the portfolio's black.
var wtTheme = map[string]any{
	"name":   "jedkx",
	"window": map[string]any{"applicationTheme": "dark", "useMica": false},
	// tabs in shades of grey: the active one lightest, like a selected line
	"tab": map[string]any{
		"background": "#262626FF", "unfocusedBackground": "#141414FF", "showCloseButton": "hover",
	},
	"tabRow": map[string]any{"background": "#0A0A0AFF", "unfocusedBackground": "#0A0A0AFF"},
}

// wtKeys are the Windows Terminal settings jedkx-cli sets: the window chrome
// (grey/black tabs; WT themes are window-wide) and the jedkx-cli profile as the
// one a new window opens with. Your other profiles keep their own colours — the
// jedkx colours live in the jedkx-cli profile. Undo gives the old default back.
// An older setup also set profiles.defaults.colorScheme; applying again gives
// it back.
func wtKeys() map[string]any {
	return map[string]any{
		"theme":                         "jedkx",
		"defaultProfile":                FragmentProfileGUID("jedkx-cli", ProfileName),
		"profiles.defaults.colorScheme": "jedkx", // every profile in the jedkx colours; undo gives yours back
	}
}

// makeDefault applies wtKeys to each WT settings file, adds the jedkx window
// theme, and records the old values. Files with comments (JSONC) are left alone.
func makeDefault(s *state) error {
	files := wtSettingsFiles()
	if len(files) == 0 {
		return errors.New("Windows Terminal settings not found — open Windows Terminal once, then run jedkx-cli setup again")
	}
	if s.PrevWT == nil {
		s.PrevWT = map[string]map[string]json.RawMessage{}
	}
	oldOurs := s.OurWT
	s.OurWT = map[string]json.RawMessage{}
	var errs []error
	for _, f := range files {
		obj, err := readObject(f)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		prev := s.PrevWT[f]
		if prev == nil {
			prev = map[string]json.RawMessage{}
			s.PrevWT[f] = prev
		}
		// keys an older setup set but this one no longer does (defaultProfile):
		// give them back, unless the user changed them since
		for k, old := range prev {
			if _, still := wtKeys()[k]; still {
				continue
			}
			if cur, ok := obj.getPath(k); ok && sameJSON(cur, oldOurs[k]) {
				if string(old) == "null" {
					obj.delPath(k)
				} else if err := obj.setPath(k, old); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", f, err))
				}
			}
			delete(prev, k)
		}
		for k, v := range wtKeys() {
			old, ok := obj.getPath(k)
			if !ok {
				old = json.RawMessage("null")
			}
			if _, seen := prev[k]; !seen || !sameJSON(old, oldOurs[k]) { // first run, or the user changed it since
				prev[k] = old
			}
			ours, _ := json.Marshal(v)
			if err := obj.setPath(k, ours); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", f, err))
			}
			s.OurWT[k] = ours
		}
		if err := upsertTheme(obj); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f, err))
		}
		if err := writeObject(f, obj); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// upsertTheme puts the jedkx entry into the "themes" list (replacing an older one).
func upsertTheme(obj *object) error {
	var list []json.RawMessage
	if cur, ok := obj.get("themes"); ok {
		if err := json.Unmarshal(cur, &list); err != nil {
			return errors.New("themes is not a list")
		}
	}
	list = withoutTheme(list)
	ours, _ := json.Marshal(wtTheme)
	b, _ := json.Marshal(append(list, ours))
	obj.set("themes", b)
	return nil
}

func withoutTheme(list []json.RawMessage) []json.RawMessage {
	out := list[:0:0]
	for _, t := range list {
		var named struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(t, &named) == nil && named.Name == "jedkx" {
			continue
		}
		out = append(out, t)
	}
	return out
}

func undoDefault(s state) error {
	var errs []error
	for f, prev := range s.PrevWT {
		if _, err := os.Stat(f); err != nil {
			continue
		}
		obj, err := readObject(f)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for k, old := range prev {
			if cur, ok := obj.getPath(k); ok && !sameJSON(cur, s.OurWT[k]) {
				continue // the user picked something else since
			}
			if string(old) == "null" {
				obj.delPath(k)
			} else if err := obj.setPath(k, old); err != nil {
				errs = append(errs, err)
			}
		}
		if cur, ok := obj.get("themes"); ok {
			var list []json.RawMessage
			if json.Unmarshal(cur, &list) == nil {
				if rest := withoutTheme(list); len(rest) == 0 {
					obj.del("themes")
				} else {
					b, _ := json.Marshal(rest)
					obj.set("themes", b)
				}
			}
		}
		dropFragmentProfile(obj) // WT copies fragment profiles into its list; take ours out
		errs = append(errs, writeObject(f, obj))
	}
	return errors.Join(errs...)
}

// dropFragmentProfile removes the entry Windows Terminal adds to profiles.list
// for our fragment profile ({"source": "jedkx-cli", …}); other entries stay.
func dropFragmentProfile(obj *object) {
	cur, ok := obj.getPath("profiles.list")
	if !ok {
		return
	}
	var list []json.RawMessage
	if json.Unmarshal(cur, &list) != nil {
		return
	}
	keep := list[:0]
	for _, raw := range list {
		var p struct {
			Source string `json:"source"`
		}
		if json.Unmarshal(raw, &p) == nil && p.Source == "jedkx-cli" {
			continue
		}
		keep = append(keep, raw)
	}
	if len(keep) == len(list) {
		return
	}
	b, _ := json.Marshal(keep)
	_ = obj.setPath("profiles.list", b)
}
