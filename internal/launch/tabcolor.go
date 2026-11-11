package launch

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
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
func RunTabColor() int {
	dir, err := os.Getwd()
	if err != nil {
		return 0
	}
	writeToClaudeConsole(tabSequence(dir))
	return 0
}
