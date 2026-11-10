# CLAUDE.md — jedkx-cli

Go CLI/TUI that lists and resumes Claude Code + Codex sessions. Public repo `github.com/jedkx/jedkx-cli`, English code/docs.
Architecture map: `README.md › Development`; platforms: `docs/PLATFORMS.md`.

## Rules

- **Read-only toward other tools.** Adapters (`internal/source/*`) never write to `~/.claude`, `~/.codex`, etc. Only exception: `Delete`, after an explicit y/n, through `source.RemoveUnder` (refuses paths outside the tool's session root).
- **`setup` is the only other writer**: it sets a few keys in `~/.claude/settings.json` (statusLine, theme, spinnerVerbs, spinnerTipsOverride, companyAnnouncements, terminalProgressBarEnabled; order-preserving), writes `~/.claude/themes/jedkx.json`, a Windows Terminal fragment (scheme + `jedkx-cli` profile = PowerShell in the jedkx colours, never auto-starts the list; only that profile uses the jedkx colours) and two WT keys (`theme` + a `jedkx` themes entry, `defaultProfile` → the jedkx-cli profile; other profiles keep their colours, undo restores the old default) — opt-in, state in `setup.json`, reversed by `setup.Undo` (called by uninstall).
- **Platforms:** see `docs/PLATFORMS.md`; build tags only in `internal/install` and `internal/launch` (process checks).
- **Owned panes:** panes opened beside the list run via `jedkx-cli _pane <pid> -- …` and close when the list exits; tabs stay independent.
- **Light.** No new dependency without a reason; no background goroutines that outlive a command; no fixed-rate tickers (one exception: an owned pane polls its owner process once a second, `internal/launch/pane.go`). Nox follows J-Portfolio's scheduler 1:1 (`HiddenResident.tsx`, `residentAnimation.ts`, `art/resident/sheet.json` durations): "notice" once on open, rest 2.2–4.6 s then the staggered `residentBlink`; without a key ≥6.5 s "search", ≥22 s "sleepy" (sleeps until a key, then "wake"), one cue per 14 s; errors play "surprised" — one timer per frame, never a frame loop.
- **Clean uninstall.** Anything new that install creates on the machine must be a manifest `Item` in
  `internal/install` and be removed + verified by `Uninstall`. Paths come only from `internal/paths`.
- **Identity in one place:** product name, repo, mascot → `internal/app`; colours → `internal/ui/theme.go`.
- Release asset names (`jedkx-cli_<os>_<arch>[.exe]`) must match `internal/update.AssetName()`, `.goreleaser.yaml` and `scripts/install.*`.
- Every rendered line must fit the terminal width (`internal/ui/view_test.go`).

## Commands

Needs Go 1.27+ on PATH (a per-user SDK install works).

```bash
gofmt -l . ; go vet ./... ; go test ./...
go build -trimpath -ldflags "-s -w -X github.com/jedkx/jedkx-cli/internal/app.Version=v0.1.0" -o dist/jedkx-cli.exe ./cmd/jedkx-cli
```

Before a release, by hand on Windows: run the exe → pick a choice → new terminal → `jedkx-cli` works; `uninstall --dry-run` lists only existing items; after uninstall the user PATH is unchanged and Settings › Apps has no entry; `update` from an older tag swaps the binary.
