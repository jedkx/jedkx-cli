// Package install puts jedkx-cli on the machine and takes it off again — completely.
//
// Every change made during install is written to a manifest (install.json).
// Uninstall replays the manifest in reverse instead of guessing, then verifies
// that each item is really gone. Data owned by other tools is never touched.
package install

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jedkx/jedkx-cli/internal/app"
	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/paths"
	"github.com/jedkx/jedkx-cli/internal/setup"
	"github.com/jedkx/jedkx-cli/internal/update"
)

// Item kinds recorded in the manifest.
const (
	KindFile     = "file"     // a file we created
	KindDir      = "dir"      // a directory we created (removed with its content)
	KindPath     = "path"     // an entry we added to the user PATH
	KindRegistry = "registry" // Windows "Installed apps" entry
)

// Item is one change made on the machine.
type Item struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

// Manifest is the install record.
type Manifest struct {
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
	Items       []Item    `json:"items"`
}

// ManifestFile lives inside the install dir, so it goes away with it.
func ManifestFile() string { return filepath.Join(paths.InstallDir(), "install.json") }

func loadManifest() (Manifest, bool) {
	var m Manifest
	b, err := os.ReadFile(ManifestFile())
	if err != nil || json.Unmarshal(b, &m) != nil {
		return m, false
	}
	return m, true
}

// InstalledVersion is the version of the installed copy ("" when none).
func InstalledVersion() string {
	if !Installed() {
		return ""
	}
	if m, ok := loadManifest(); ok && m.Version != "" {
		return m.Version
	}
	return "unknown"
}

// Installed reports whether an installed copy exists.
func Installed() bool {
	_, err := os.Stat(paths.InstalledExe())
	return err == nil
}

// RunningInstalled reports whether this process IS the installed copy.
func RunningInstalled() bool { return paths.SamePath(paths.Self(), paths.InstalledExe()) }

// Install copies the running binary into place, puts it on PATH and registers it.
// Safe to run again (repairs a broken install).
func Install(out io.Writer) error {
	src, dst := paths.Self(), paths.InstalledExe()
	if src == "" {
		return errors.New("cannot locate the running binary")
	}
	m := Manifest{Version: app.Version, InstalledAt: time.Now()}

	if err := os.MkdirAll(paths.InstallDir(), 0o755); err != nil {
		return err
	}
	m.Items = append(m.Items, Item{KindDir, paths.InstallDir()})
	if err := os.MkdirAll(paths.BinDir(), 0o755); err != nil {
		return err
	}
	if !paths.SamePath(src, dst) {
		if err := copyBinary(src, dst); err != nil {
			return fmt.Errorf("copy binary: %w", err)
		}
	}
	m.Items = append(m.Items, Item{KindFile, dst})
	fmt.Fprintf(out, "  ✓ binary      %s\n", dst)

	added, err := addToPath(paths.BinDir())
	switch {
	case err != nil:
		fmt.Fprintf(out, "  ! PATH        %v\n", err)
	case added:
		m.Items = append(m.Items, Item{KindPath, paths.BinDir()})
		fmt.Fprintf(out, "  ✓ PATH        %s\n", paths.BinDir())
	default:
		fmt.Fprintf(out, "  ✓ PATH        already contains %s\n", paths.BinDir())
	}
	if prev, ok := loadManifest(); ok && hasItem(prev, KindPath) && !added {
		m.Items = append(m.Items, Item{KindPath, paths.BinDir()}) // keep ownership across reinstalls
	}

	if key, err := registerApp(dst); err != nil {
		fmt.Fprintf(out, "  ! apps list   %v\n", err)
	} else if key != "" {
		m.Items = append(m.Items, Item{KindRegistry, key})
		fmt.Fprintf(out, "  ✓ apps list   %s\n", "Settings › Apps › Installed apps")
	}

	b, _ := json.MarshalIndent(m, "", "  ")
	return config.WriteAtomic(ManifestFile(), b)
}

func hasItem(m Manifest, kind string) bool {
	for _, it := range m.Items {
		if it.Kind == kind {
			return true
		}
	}
	return false
}

// copyBinary writes dst via a temp file; a running dst is moved aside first (Windows).
func copyBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, in); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		if _, err := update.MoveAside(dst); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return os.Rename(tmp, dst)
}

// Options controls uninstall.
type Options struct {
	DryRun bool // show the plan, change nothing
	Purge  bool // also delete the user's own data without asking
	Yes    bool // no questions (used by Settings › Apps); personal data is KEPT unless Purge
}

type step struct {
	label string
	run   func() error
	gone  func() bool // verification
}

// Uninstall removes everything install created, plus caches; personal data on consent.
func Uninstall(opt Options, in io.Reader, out io.Writer) error {
	m, ok := loadManifest()
	if !ok { // no manifest: fall back to the known default layout
		m.Items = []Item{{KindDir, paths.InstallDir()}, {KindFile, paths.InstalledExe()}}
		if pathHas(paths.BinDir()) {
			m.Items = append(m.Items, Item{KindPath, paths.BinDir()})
		}
		if key := registryKey(); key != "" && registryExists() {
			m.Items = append(m.Items, Item{KindRegistry, key})
		}
	}

	var steps []step
	var selfDir string
	for i := len(m.Items) - 1; i >= 0; i-- {
		it := m.Items[i]
		switch it.Kind {
		case KindRegistry:
			steps = append(steps, step{"apps list entry", unregisterApp, func() bool { return !registryExists() }})
		case KindPath:
			dir := it.Target
			steps = append(steps, step{"PATH entry  " + dir, func() error { return removeFromPath(dir) }, func() bool { return !pathHas(dir) }})
		case KindFile:
			f := it.Target
			if !paths.SamePath(f, paths.InstalledExe()) {
				continue // safety: only ever delete our own binary
			}
			if runtime.GOOS == "windows" && paths.SamePath(paths.Self(), f) {
				// A running exe cannot delete itself on Windows. It lives inside the
				// install dir (KindDir), which is removed right after exit.
				continue
			}
			steps = append(steps, fileStep(f), fileStep(f+".old"), fileStep(f+".new"))
		case KindDir:
			d := it.Target
			if !paths.SamePath(d, paths.InstallDir()) {
				continue // safety: only ever delete our own install dir
			}
			if isInside(paths.Self(), d) {
				selfDir = d
				continue
			}
			steps = append(steps, step{"folder      " + d, func() error { return os.RemoveAll(d) }, func() bool { return !exists(d) }})
		}
	}
	steps = append(steps, step{"jedkx look  Claude Code status line + theme, Windows Terminal profile (put back as before)",
		func() error { return setup.Undo(io.Discard) },
		func() bool { c, t := setup.Applied(); return !c && !t }})
	cache := paths.CacheDir()
	steps = append(steps, step{"cache       " + cache, func() error { return os.RemoveAll(cache) }, func() bool { return !exists(cache) }})

	present := steps[:0]
	for _, s := range steps { // only list what is really there
		if !s.gone() {
			present = append(present, s)
		}
	}
	steps = present

	purge := opt.Purge
	data := paths.ConfigDir()
	if len(steps) == 0 && selfDir == "" && !exists(data) {
		fmt.Fprintln(out, "jedkx-cli is not installed — nothing to remove.")
		return nil
	}
	fmt.Fprintln(out, "jedkx-cli will remove:")
	for _, s := range steps {
		fmt.Fprintln(out, "  -", s.label)
	}
	if selfDir != "" {
		fmt.Fprintln(out, "  - folder     ", selfDir, "(right after exit)")
	}
	if exists(data) {
		if purge {
			fmt.Fprintln(out, "  - your data  ", data)
		} else {
			fmt.Fprintln(out, "  ? your data  ", data, "(names, pins, settings — asked below)")
		}
	}
	fmt.Fprintln(out, "Sessions in ~/.claude and ~/.codex are never touched.")
	if opt.DryRun {
		fmt.Fprintln(out, "\nDry run — nothing was changed.")
		return nil
	}

	r := bufio.NewReader(in)
	if !opt.Yes {
		if !ask(r, out, "\nUninstall jedkx-cli? [y/N] ") {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
		if !purge && exists(data) {
			purge = ask(r, out, "Also delete your data (names, pins, settings)? [y/N] ")
		}
	}
	if purge {
		steps = append(steps, step{"your data   " + data, func() error {
			if c, t := setup.Applied(); c || t { // undo failed: keep setup.json, it holds the old values
				return errors.New("kept: the jedkx look could not be undone, its saved values are in setup.json")
			}
			return os.RemoveAll(data)
		}, func() bool { return !exists(data) }})
	}

	var left []string
	for _, s := range steps {
		if err := s.run(); err != nil && !errors.Is(err, os.ErrNotExist) {
			left = append(left, fmt.Sprintf("%s (%v)", s.label, err))
			continue
		}
		if !s.gone() {
			left = append(left, s.label)
		}
	}
	if selfDir != "" {
		if err := scheduleRemoval(selfDir); err != nil {
			left = append(left, fmt.Sprintf("folder %s (%v)", selfDir, err))
		}
	}
	broadcastEnvChange()

	if len(left) > 0 {
		fmt.Fprintln(out, "\nCould not remove:")
		for _, l := range left {
			fmt.Fprintln(out, "  ✗", l)
		}
		return errors.New("uninstall incomplete")
	}
	fmt.Fprintln(out, "\n✓ jedkx-cli is gone. Nothing left behind.")
	if !purge && exists(data) {
		fmt.Fprintln(out, "  Your data was kept at", data, "— delete it any time.")
	}
	return nil
}

func fileStep(f string) step {
	return step{"file        " + f, func() error { return os.Remove(f) }, func() bool { return !exists(f) }}
}

func ask(r *bufio.Reader, out io.Writer, q string) bool {
	fmt.Fprint(out, q)
	s, _ := r.ReadString('\n')
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "y" || s == "yes" || s == "e" || s == "evet"
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// isInside reports whether p is dir or below it.
func isInside(p, dir string) bool {
	if p == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

// SyncVersion runs on start of the installed copy: after a self-update the binary
// is newer than what install recorded, so the manifest and the Settings › Apps
// entry are brought up to date. Cheap when nothing changed (one small read).
func SyncVersion() {
	if app.Version == "dev" || !RunningInstalled() {
		return
	}
	m, ok := loadManifest()
	if !ok || m.Version == app.Version {
		return
	}
	m.Version = app.Version
	for _, it := range m.Items {
		if it.Kind == KindRegistry {
			setDisplayVersion(app.Version)
		}
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	_ = config.WriteAtomic(ManifestFile(), b)
}
