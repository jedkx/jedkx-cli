<div align="center">

# jedkx-cli

**Your AI coding chats, one keystroke away.**
A light terminal home for Claude Code and Codex — every chat of every project in one list.
Watched over by **Nox**, the hidden resident.

<img src="docs/screenshots/list.svg" alt="jedkx-cli: chats grouped by project, Nox in the header" width="100%">

</div>

## Why this exists

Claude Code and Codex keep every conversation on disk, and that is the problem: they keep *a lot* of
them. After a few weeks you have hundreds of chats spread over a dozen folders. The one you need —
"the refund thing from Tuesday" — is behind a `--resume` picker that only shows the current folder,
titled by whatever the first prompt happened to be.

So you start over. You re-explain the codebase, re-paste the error, re-decide what was already decided.
The context you paid for is still on disk; you just can't find it.

jedkx-cli is built around that gap: **one list of every chat, from every tool, in every project**, with
search, names and pins — and Enter drops you back into the conversation, beside the list, where you left it.

## Why it reads the logs instead of keeping its own

A session manager is tempting to build as a database: import chats, index them, keep them in sync.
That copy drifts the moment you run `claude` without it, and it is one more place your code and prompts live.

jedkx-cli keeps nothing of yours but names and pins. It reads the tools' own files, read-only:

- **Sampled, not loaded.** A chat log can be hundreds of MB. Each file is read from its head (folder,
  branch, first prompt) and its tail (latest title), never whole.
- **Cached by size + mtime.** Unchanged files are not opened again; a warm start of hundreds of chats
  takes about 0.1 s.
- **Resumed by the tool itself.** Enter runs `claude --resume <id>` / `codex resume <id>` in the chat's own
  folder — jedkx-cli never re-implements a conversation.

The one exception is `d` (delete), after a y/n, and only inside the tool's own session folder.

## What it does

- **One list** — Claude Code + Codex, grouped by project (same-named folders show their parent), pinned on top.
- **Resume anywhere** — beside the list in a split pane (`enter`), full screen (`o`) or in a new tab (`t`).
  Panes opened beside the list close with it.
- **Find it** — search titles, first prompts, projects, branches and Jira keys (Turkish-aware: `islem` finds
  `İŞLEM`); `ctrl+f` searches inside every chat.
- **See before you open** — `space` previews the last messages.
- **Keep it tidy** — rename, pin, archive, sort; select a chat, a whole folder or everything (`x`, `ctrl+a`) and
  delete, archive, pin or export in one go.
- **Take it with you** — `e` exports a chat as Markdown; `c` copies the resume command; `f` opens the folder.
- **Your instructions in one place** — `tab` to **skills & md** lists skills, `CLAUDE.md` / `AGENTS.md`, agents and commands,
  globally and per project.
- **Usage** — `jedkx-cli stats` sums tokens per day from Claude Code's own logs.
- **The jedkx look, if you want it** — a Claude Code theme and status line, and Windows Terminal colours
  and grey tabs. Opt-in, backed up first, undone with one command.

## Screenshots

All screenshots are rendered from the real UI with demo data (`internal/ui/shots_test.go`).

**Preview** — the last messages of the selected chat, without opening it.

<img src="docs/screenshots/preview.svg" alt="Preview of the last messages" width="100%">

**Select** — a whole folder with `x` on its header; `d a p e` act on everything selected.

<img src="docs/screenshots/select.svg" alt="Four chats selected across two projects" width="100%">

**Library** — every skill and instruction file your tools read.

<img src="docs/screenshots/library.svg" alt="Skills, CLAUDE.md, agents and commands" width="100%">

**Beside a chat** — narrow, the list says how to resize it and move between panes.

<img src="docs/screenshots/narrow.svg" alt="The list as a narrow pane" width="340">

**Claude Code status line** — folder, branch with changes and ahead/behind, Jira key, a context bar that
warms from green to red, lines changed, time, model.

<img src="docs/screenshots/statusline.svg" alt="The status line at 34, 63 and 88 percent context" width="100%">

**Keys** — `?` anywhere.

<img src="docs/screenshots/help.svg" alt="Help screen" width="100%">

## Install

**Windows** — download `jedkx-cli_windows_amd64.exe` from [Releases](https://github.com/jedkx/jedkx-cli/releases/latest)
and double-click it. It asks one thing:

| Choice | Lists | Changes |
|---|---|---|
| everything | Claude Code + Codex chats | Claude Code look, Windows Terminal look |
| Claude only | Claude Code chats | Claude Code look |
| Codex only | Codex chats | nothing else |
| terminal only | all chats | a `jedkx-cli` Windows Terminal profile (default), grey tabs |
| just the app | all chats | nothing else |

Every setting it touches is backed up next to the downloaded file first, keeping its folder layout
(`jedkx-cli-backup-<time>\.claude\settings.json`, …); at the end it asks whether to keep the backup.
Errors are printed and the window waits — it never closes on its own. Once installed, the same exe offers to
**update** it (when the file is newer) and **uninstall** as the last choice.

Or in PowerShell / a shell:

```powershell
irm https://raw.githubusercontent.com/jedkx/jedkx-cli/main/scripts/install.ps1 | iex
```

```sh
curl -fsSL https://raw.githubusercontent.com/jedkx/jedkx-cli/main/scripts/install.sh | sh
```

Per-user, **no admin / sudo**. Every download is verified against the release's SHA-256 checksums.
Open a new terminal and type `jedkx-cli`.

## Use

```
jedkx-cli                     the chat list
jedkx-cli .                   only this folder's chats
jedkx-cli last                continue the latest chat started in this folder
jedkx-cli stats [--days 7]    token use per day (Claude Code)
jedkx-cli sessions [query]    print chats (--json, --tool claude|codex, --all)
jedkx-cli setup               the jedkx look (--only claude|terminal, --undo)
jedkx-cli config [key [on|off]]   update.check · update.auto · animation · claude · codex
jedkx-cli update [--check]    install the latest release
jedkx-cli uninstall           remove everything (--dry-run, --purge, --yes)
jedkx-cli nox                 say hi to Nox
```

| Key | Action |
|---|---|
| `enter` | open beside the list (Windows Terminal / tmux) — on a project: fold / unfold |
| `o` · `t` · `n` | open full screen · in a new tab · a new chat in the same folder |
| `space` · `s` | preview the last messages · sort: recent → name → project → size |
| `/` · `ctrl+f` | search · then search inside every chat |
| `←` `→` · `z` · `v` | fold / unfold a project · all projects · flat list |
| `x` · `ctrl+a` | select a chat (on a project: the whole folder) · select all — `d` `a` `p` `e` then act on the selection |
| `p` `r` `a` | pin · rename · archive (the **archived** tab shows them; `a` there brings one back) |
| `c` · `f` · `e` | copy the resume command · open the folder · export as Markdown (Downloads) |
| `d` | delete the chat's files (asks y/n) |
| `tab` | tabs: all · claude · codex · archived │ skills & md |
| `ctrl+r` · `?` · `q` | rescan · help · quit |
| `alt+shift+←→` · `alt+←→` | Windows Terminal: resize the pane · move between panes |

## The jedkx look

`jedkx-cli setup` (or the installer) — `setup --undo` and uninstall put everything back. Values you change
later are yours: undo leaves them alone, and a second `setup` remembers your latest choice.

- **Claude Code** — a `jedkx` theme (red on black, greys, terminal green, blue/red diffs), the status line,
  Nox spinner words, jedkx-cli tips, a startup line; the tab progress signal is turned off so Windows
  Terminal keeps the grey tabs.
- **Windows Terminal** — a `jedkx-cli` profile in the `jedkx` colours (settings fragment), made the default
  profile, and grey tabs. **Your other profiles keep their colours;** undo makes your old default the default again.

The status line by hand, in `~/.claude/settings.json`:

```json
"statusLine": { "type": "command", "command": "jedkx-cli statusline" }
```

## Clean uninstall

```
jedkx-cli uninstall --dry-run     # see exactly what will be removed, change nothing
jedkx-cli uninstall               # remove; asks before deleting your own data
```

Or **Settings › Apps › Installed apps › jedkx-cli**. Install records every change in a manifest; uninstall
reverses exactly that list, undoes the jedkx look, then checks each item is gone.

| Installed | Removed |
|---|---|
| `%LOCALAPPDATA%\Programs\jedkx-cli\` (binary + manifest) | always |
| your **user** PATH entry (only that entry; system PATH never touched) | always |
| Settings › Apps entry (`HKCU`, no admin) | always |
| scan cache `%LOCALAPPDATA%\jedkx-cli` | always |
| the jedkx look (Claude Code, Windows Terminal) | always — put back as it was |
| your names / pins / settings `%APPDATA%\jedkx-cli` | only if you say yes (or `--purge`) |

## Privacy

No telemetry, no account, no server. Chats are read locally and shown on screen — never copied or sent.
The only network calls go to GitHub: the release check on start (`jedkx-cli config update.check off`) and,
with `update.auto` on (the default), downloading a newer release, checked against its SHA-256 sum
(`jedkx-cli config update.auto off`).

## Stack

- **Go 1.27**, one static binary (~9 MB), no runtime.
- **Bubble Tea** + **Lip Gloss** for the TUI; `zstd` for compressed Codex archives.
- Windows Terminal (`wt.exe`) and tmux for panes and tabs; Windows, macOS and Linux in CI.

## Architecture

```
cmd/jedkx-cli        entry point, installer menu and subcommands (stdlib flag)
internal/app         name, version, mascot — product identity in one place
internal/paths       every directory jedkx-cli uses (install/uninstall read from here)
internal/source      Session, adapter interface, scan cache, line sampling
  claude/            Claude Code (~/.claude/projects/*/*.jsonl)
  codex/             Codex (~/.codex/sessions/**/rollout-*.jsonl[.zst])
internal/catalog     adapter registry, parallel scan, merge, search, sort
internal/meta        your names / pins / archive (jedkx-cli's own file)
internal/library     skills, CLAUDE.md / AGENTS.md, agents, commands (read-only)
internal/export      a chat as Markdown
internal/stats       token use per day
internal/launch      beside / tab: Windows Terminal or tmux; clipboard; open folder
internal/ui          Bubble Tea TUI — model, keys, rows, view, library, nox, theme
internal/statusline  Claude Code status line
internal/setup       the opt-in jedkx look, backup and undo
internal/update      GitHub Releases check + verified self-replace
internal/install     manifest-based install / uninstall, PATH, Apps entry
```

**Adding a tool** is one package implementing `source.Adapter` and one line in `catalog.Adapters()`.
Platform notes and how to add a terminal: [`docs/PLATFORMS.md`](docs/PLATFORMS.md).

## Build

```sh
go test ./...
go build -trimpath -ldflags "-s -w" ./cmd/jedkx-cli
JEDKX_SHOTS=docs/screenshots go test ./internal/ui -run TestScreenshots -count=1   # refresh the screenshots
```

Releases: push a `v*` tag — GitHub Actions runs GoReleaser (Windows / macOS / Linux, amd64 + arm64, checksums).
Package managers (scoop, Homebrew, winget) are commented out in `.goreleaser.yaml` until their repos exist.

Before a release, by hand on Windows:

- [ ] double-click the exe → pick a choice → new terminal → `jedkx-cli` works
- [ ] `jedkx-cli uninstall --dry-run` lists only existing items
- [ ] after uninstall the user PATH is byte-identical to before install, and Settings › Apps has no entry
- [ ] `jedkx-cli update` on an older tag swaps the binary; `.old` is gone on the next start

## Notes

- Documentation and user-visible strings are English; keep them that way in contributions.
- Codex token use is not shown yet: its logs on the machines tested had no usage records.

## License

MIT © Caner Coşkun ([jedkx](https://github.com/jedkx)). Claude Code and Codex are their makers' products;
jedkx-cli only reads the files they write.
