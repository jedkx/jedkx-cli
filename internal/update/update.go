// Package update checks GitHub Releases and replaces the running binary safely.
// Network use: one small API call per start, in the background (can be turned off).
package update

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jedkx/jedkx-cli/internal/app"
	"github.com/jedkx/jedkx-cli/internal/config"
	"github.com/jedkx/jedkx-cli/internal/paths"
)

// State is the cached result of the last check.
type State struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

func stateFile() string { return filepath.Join(paths.CacheDir(), "update.json") }

// Cached returns the last known latest version without touching the network.
func Cached() State {
	var s State
	if b, err := os.ReadFile(stateFile()); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// Notice returns "v0.2.0" when a newer version is known, else "".
func Notice() string {
	s := Cached()
	if s.Latest != "" && Newer(s.Latest, app.Version) {
		return s.Latest
	}
	return ""
}

// Check asks GitHub for the latest release and stores the result.
func Check(timeout time.Duration) (string, error) {
	rel, err := latest(timeout)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(State{CheckedAt: time.Now(), Latest: rel.Tag})
	_ = config.WriteAtomic(stateFile(), b)
	return rel.Tag, nil
}

type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (r release) asset(name string) string {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL
		}
	}
	return ""
}

func latest(timeout time.Duration) (release, error) {
	var r release
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+app.Repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", app.Name+"/"+app.Version)
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return r, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return r, fmt.Errorf("GitHub: HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(resp.Body).Decode(&r)
	return r, err
}

// AssetName is the release file for this platform (GoReleaser "binary" format).
func AssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("%s_%s_%s%s", app.Name, runtime.GOOS, runtime.GOARCH, ext)
}

// Apply downloads the latest release, verifies its SHA-256 and swaps the binary.
// The old binary is kept as <exe>.old until the next start, so a failed swap can roll back.
func Apply(progress func(string)) (string, error) {
	rel, err := latest(15 * time.Second)
	if err != nil {
		return "", err
	}
	if !Newer(rel.Tag, app.Version) {
		return "", ErrUpToDate
	}
	binURL, sumURL := rel.asset(AssetName()), rel.asset("checksums.txt")
	if binURL == "" || sumURL == "" {
		return "", fmt.Errorf("release %s has no %s or checksums.txt", rel.Tag, AssetName())
	}
	progress("checksums")
	want, err := checksum(sumURL, AssetName())
	if err != nil {
		return "", err
	}
	exe := paths.Self()
	// a name of its own: two jedkx-cli updating at once (two lists open) must not
	// write into the same file — the last rename wins and both files are whole
	tmp := fmt.Sprintf("%s.new-%d", exe, os.Getpid())
	progress("downloading " + rel.Tag)
	got, err := download(binURL, tmp)
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if !strings.EqualFold(got, want) {
		os.Remove(tmp)
		return "", errors.New("checksum mismatch — update aborted, nothing changed")
	}
	progress("installing")
	old, err := MoveAside(exe)
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, exe); err != nil {
		_ = os.Rename(old, exe) // roll back
		return "", err
	}
	b, _ := json.Marshal(State{CheckedAt: time.Now(), Latest: rel.Tag})
	_ = config.WriteAtomic(stateFile(), b)
	return rel.Tag, nil
}

// ErrUpToDate means there is nothing to install.
var ErrUpToDate = errors.New("already up to date")

// Cleanup removes leftovers of earlier updates (<exe>.old*, <exe>.new*). Files still
// in use by an older jedkx-cli that is running are skipped and go next time.
func Cleanup() {
	exe := paths.Self()
	if exe == "" {
		return
	}
	olds, _ := filepath.Glob(exe + ".old*")
	news, _ := filepath.Glob(exe + ".new*")
	for _, f := range append(olds, news...) {
		os.Remove(f)
	}
}

// MoveAside renames exe out of the way so a new binary can take its place.
// Windows allows renaming a running exe but not replacing one that is still in
// use, so when <exe>.old is held by another running jedkx-cli a unique name is used.
func MoveAside(exe string) (string, error) {
	old := exe + ".old"
	if os.Remove(old) != nil {
		if _, err := os.Stat(old); err == nil {
			old = fmt.Sprintf("%s.old-%d", exe, time.Now().UnixNano())
		}
	}
	return old, os.Rename(exe, old)
}

func checksum(url, name string) (string, error) {
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[1] == name {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", name)
}

func download(url, dst string) (string, error) {
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return hex.EncodeToString(h.Sum(nil)), err
}

// Newer reports whether version a is greater than b ("v1.2.3"; "dev" is never newer).
func Newer(a, b string) bool {
	pa, oka := parse(a)
	pb, okb := parse(b)
	if !oka {
		return false
	}
	if !okb {
		return true
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
