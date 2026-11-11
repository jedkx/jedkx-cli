package launch

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jedkx/jedkx-cli/internal/meta"
	"github.com/jedkx/jedkx-cli/internal/source/claude"
)

// Tab colour per project: Claude Code runs `jedkx-cli _tabcolor` when a chat
// starts (a SessionStart hook that setup adds). It picks one of a few greys from
// the project's name and paints the Windows Terminal tab with it, so tabs of
// different projects are told apart at a glance and still look jedkx-grey.

// TabColorCommand is the hidden subcommand the hook runs.
const TabColorCommand = "_tabcolor"

// tabGreys are the tab colours, close greys with a hint of warmth; OSC 4 writes
// them into palette slots 200+ (rarely used) and DECAC gives the tab that slot.
var tabGreys = []string{"2b/2b/2b", "36/34/34", "41/3d/3d", "4b/48/48", "30/30/33"}

const tabSlot = 200

// doneColour marks a tab whose chat has finished and waits for you (the Stop
// hook); your next message (UserPromptSubmit) gives the project grey back.
const (
	doneColour = "5a/1e/1e"
	doneSlot   = 206
)

func doneSequence() string {
	return fmt.Sprintf("\x1b]4;%d;rgb:%s\x1b\\\x1b[2;15;%d,|", doneSlot, doneColour, doneSlot)
}

// tabSequence is what goes to the terminal for this project folder.
func tabSequence(dir string) string {
	root := dir
	for d := dir; d != "" && d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			root = d
			break
		}
	}
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(filepath.Base(root))))
	i := int(h.Sum32() % uint32(len(tabGreys)))
	n := tabSlot + i
	return fmt.Sprintf("\x1b]4;%d;rgb:%s\x1b\\\x1b[2;15;%d,|", n, tabGreys[i], n)
}

// RunTabColor is the hook's body; it never fails (a hook must not break a chat).
// "done" (the Stop hook) marks the tab as finished.
func RunTabColor(args []string) int {
	title := tabTitle()
	if len(args) > 0 && args[0] == "done" {
		writeToClaudeConsole(title + doneSequence())
		return 0
	}
	dir, err := os.Getwd()
	if err != nil {
		return 0
	}
	writeToClaudeConsole(title + tabSequence(dir))
	return 0
}

// tabTitle is the OSC 0 sequence that names a tab jedkx-cli opened after its
// chat ("" elsewhere: Claude names its own tabs there). The name is the one
// the list shows: yours (r) first, then Claude's title, then the first prompt.
func tabTitle() string {
	if os.Getenv(TabKeyEnv) == "" {
		return ""
	}
	var in struct {
		SessionID      string `json:"session_id"`
		TranscriptPath string `json:"transcript_path"`
	}
	done := make(chan struct{})
	go func() { // Claude sends the hook's input and closes stdin; never wait long
		_ = json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&in)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		return ""
	}
	name := ""
	if in.SessionID != "" {
		name = meta.Load(meta.File()).Get("claude:" + in.SessionID).Name
	}
	if name == "" && in.TranscriptPath != "" {
		if s, ok := claude.ParseFile(in.TranscriptPath); ok {
			name = s.Label()
		}
	}
	name = strings.Map(func(r rune) rune { // no control characters in a title
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if r := []rune(name); len(r) > 40 {
		name = string(r[:39]) + "…"
	}
	shown := name
	if shown == "" {
		shown = os.Getenv(tabTitleEnv) // not named yet: the tab keeps its first title
	}
	if in.SessionID != "" {
		pid, _ := strconv.Atoi(os.Getenv(tabPIDEnv))
		claim("claude:"+in.SessionID, pid, shown, os.Getenv(tabWhereEnv))
	}
	if name == "" {
		return ""
	}
	return "\x1b]0;" + name + "\x07"
}
