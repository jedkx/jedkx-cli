// Package account tells which account Claude Code and Codex are signed in
// with, so you can see it at a glance when you switch between accounts. It
// reads only the e-mail, read-only, from the tools' own files; tokens are never
// read into anything that is kept or shown.
package account

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/jedkx/jedkx-cli/internal/paths"
)

// Claude is the e-mail Claude Code is signed in with ("" when unknown).
// Claude Code keeps it in ~/.claude.json (or $CLAUDE_CONFIG_DIR/.claude.json).
func Claude() string {
	f := filepath.Join(paths.Home(), ".claude.json")
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		f = filepath.Join(d, ".claude.json")
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return ""
	}
	var c struct {
		OAuth struct {
			Email string `json:"emailAddress"`
		} `json:"oauthAccount"`
	}
	_ = json.Unmarshal(b, &c)
	return c.OAuth.Email
}

// Codex is the e-mail Codex is signed in with ("" when unknown or an API key).
// It is the "email" claim of the id_token in ~/.codex/auth.json.
func Codex() string {
	h := os.Getenv("CODEX_HOME")
	if h == "" {
		h = filepath.Join(paths.Home(), ".codex")
	}
	b, err := os.ReadFile(filepath.Join(h, "auth.json"))
	if err != nil {
		return ""
	}
	var a struct {
		Tokens struct {
			ID string `json:"id_token"`
		} `json:"tokens"`
	}
	if json.Unmarshal(b, &a) != nil {
		return ""
	}
	return jwtEmail(a.Tokens.ID)
}

// jwtEmail reads the "email" claim of a JWT without verifying it (display only).
func jwtEmail(tok string) string {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return ""
	}
	p, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return ""
	}
	var c struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(p, &c)
	return c.Email
}
