package account

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestAccounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if Claude() != "" || Codex() != "" {
		t.Fatal("no files: no account")
	}
	_ = os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"you@example.com"}}`), 0o600)
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"dev@example.com"}`))
	_ = os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".codex", "auth.json"), []byte(`{"tokens":{"id_token":"h.`+payload+`.s"}}`), 0o600)
	if got := Claude(); got != "you@example.com" {
		t.Fatalf("claude %q", got)
	}
	if got := Codex(); got != "dev@example.com" {
		t.Fatalf("codex %q", got)
	}
}
