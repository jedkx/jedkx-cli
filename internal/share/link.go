package share

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jedkx/jedkx-cli/internal/paths"
)

// linkDir makes link point at the directory target. On Windows it is a
// junction (no admin, no Developer Mode); elsewhere a symlink.
func linkDir(link, target string) error {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err != nil {
			return errors.New(strings.TrimSpace(string(out)))
		}
		return nil
	}
	return os.Symlink(target, link)
}

// linkFile makes link share the file target: a symlink where allowed, else a
// hard link (Windows without Developer Mode). It returns which one it made.
func linkFile(link, target string) (string, error) {
	if err := os.Symlink(target, link); err == nil {
		return "symlink", nil
	}
	if err := os.Link(target, link); err != nil {
		return "", err
	}
	return "hardlink", nil
}

// pointsAt reports whether p is a link (junction / symlink) to target, or —
// for files — the same file as target (hard link).
func pointsAt(p, target string) bool {
	if dst, err := os.Readlink(p); err == nil {
		dst = strings.TrimPrefix(dst, `\??\`)
		if !filepath.IsAbs(dst) {
			dst = filepath.Join(filepath.Dir(p), dst)
		}
		return paths.SamePath(dst, target)
	}
	a, err1 := os.Stat(p)
	b, err2 := os.Stat(target)
	return err1 == nil && err2 == nil && !a.IsDir() && os.SameFile(a, b)
}

// isLink reports whether p is a symlink or junction (never followed).
func isLink(p string) bool {
	_, err := os.Readlink(p)
	return err == nil
}

// unlink removes the link p itself, never what it points at.
func unlink(p string) error {
	if !isLink(p) {
		return errors.New("not a link: " + p)
	}
	return os.Remove(p)
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// copyTree copies the directory src into dst (created), files and folders only.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil // links inside skills are left out
		}
		return copyFile(p, to)
	})
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// sameTree reports whether two folders hold the same files with the same bytes.
func sameTree(a, b string) bool {
	files := func(root string) map[string]string {
		m := map[string]string{}
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				rel, _ := filepath.Rel(root, p)
				c, _ := os.ReadFile(p)
				m[filepath.ToSlash(rel)] = string(c)
			}
			return nil
		})
		return m
	}
	x, y := files(a), files(b)
	if len(x) != len(y) {
		return false
	}
	for k, v := range x {
		if y[k] != v {
			return false
		}
	}
	return true
}
