# Platforms

jedkx-cli is built and tested on Windows, Linux and macOS in CI
(`.github/workflows/ci.yml`, matrix `windows-latest`, `ubuntu-latest`,
`macos-latest`). You do not need a Mac or a Linux box to keep those working:
push a branch and read the CI run.

## What works where

| Feature | Windows | Linux | macOS |
|---|---|---|---|
| List / search / pin / rename / archive / delete | ✓ | ✓ | ✓ |
| Open here (`o`) | ✓ | ✓ | ✓ |
| Open beside (`enter`) · new tab (`t`) | Windows Terminal, tmux | tmux | tmux |
| Status line for Claude Code | ✓ | ✓ | ✓ |
| `setup`: Claude Code theme + status line | ✓ | ✓ | ✓ |
| `setup`: terminal colour scheme + profile | Windows Terminal | — | — |
| `install` | `%LOCALAPPDATA%\Programs`, user PATH, Apps entry | `~/.local/bin` | `~/.local/bin` |
| Self-update | ✓ | ✓ | ✓ (unsigned: Gatekeeper may ask once) |

Without tmux on Linux/macOS, `enter` opens the session in place and you come
back to the list when it ends.

## Where the platform code lives

| Concern | File | How to extend |
|---|---|---|
| Panes and tabs | `internal/launch/launch.go` | add a `HostKind`, detect it in `Host()` (env var the terminal sets), add its split/tab commands. Good next ones: WezTerm (`WEZTERM_PANE` → `wezterm cli split-pane`), kitty (`KITTY_WINDOW_ID` → `kitty @ launch`), zellij (`ZELLIJ` → `zellij run`), iTerm2 (AppleScript). |
| Install, PATH, uninstall | `internal/install/install_windows.go`, `install_other.go` | Unix never edits shell profiles; it installs to `~/.local/bin` and says so when that is not on PATH. |
| Terminal look | `internal/setup/setup.go › Terminal()` | Windows Terminal uses a settings fragment. For others, write the terminal's own include/theme file (e.g. a WezTerm or kitty colour file) and record it so `Undo()` removes it. |
| Directories | `internal/paths` | all paths come from here (`os.UserConfigDir`, `os.UserCacheDir`). |
| Release assets | `.goreleaser.yaml`, `internal/update.AssetName()`, `scripts/install.sh` | names must match: `jedkx-cli_<os>_<arch>[.exe]`. |

## Rules for platform work

- Build tags (`//go:build windows` / `!windows`) only in `internal/install` and `internal/launch` (`proc_*.go`); everywhere
  else branch on `runtime.GOOS` or on what the environment offers.
- Anything written to the machine is recorded so uninstall / `setup --undo` reverses it.
- Run `GOOS=linux go vet ./...` and `GOOS=darwin go vet ./...` locally before pushing.
