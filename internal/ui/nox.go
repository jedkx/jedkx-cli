package ui

import (
	"math/rand"
	"strings"
	"time"
)

// Nox — the hidden resident. Ported 1:1 from J-Portfolio
// (src/assets/residentBlockFrames.ts) so the character stays one design.

const (
	noxW = 34
	noxH = 12
)

type sprite []string

var (
	eyeOpen   = sprite{" ▄████▄ ", "███  ███", "███  ███", " ▀████▀ "}
	eyeLeft   = sprite{" ▄████▄ ", "██  ████", "██  ████", " ▀████▀ "}
	eyeRight  = sprite{" ▄████▄ ", "████  ██", "████  ██", " ▀████▀ "}
	eyeHalf   = sprite{"        ", "▄▄▄▄▄▄▄▄", "▀██  ██▀", "        "}
	eyeClosed = sprite{"        ", "        ", "▄▄▄▄▄▄▄▄", "        "}
	eyeHappy  = sprite{"        ", " ▄████▄ ", "██▀  ▀██", "        "}
	eyeWide   = sprite{" ▄████▄ ", "██    ██", "██    ██", " ▀████▀ "}

	mouthSmile = sprite{"▄      ▄", " ▀▄▄▄▄▀ "}
	mouthSmall = sprite{"        ", "  ▄▄▄▄  "}
	mouthOpen  = sprite{" ▄████▄ ", "████████", " ▀████▀ "}
	mouthChew  = sprite{"        ", " ▄▄▄▄▄▄ "}
)

func pose(left, right, mouth sprite) []string {
	grid := make([][]rune, noxH)
	for y := range grid {
		grid[y] = []rune(strings.Repeat(" ", noxW))
	}
	stamp := func(s sprite, x, y int) {
		for row, line := range s {
			for col, r := range []rune(line) {
				if r != ' ' {
					grid[y+row][x+col] = r
				}
			}
		}
	}
	stamp(left, 3, 3)
	stamp(right, 23, 3)
	stamp(mouth, 13, 8)
	out := make([]string, noxH)
	for i, r := range grid {
		out[i] = strings.TrimRight(string(r), " ")
	}
	// top rows are empty in the original canvas; keep only rows 2..10 to save space
	return out[2:11]
}

type frame struct {
	d     time.Duration
	lines []string
}

func f(ms int, l, r, m sprite) frame {
	return frame{time.Duration(ms) * time.Millisecond, pose(l, r, m)}
}

// idle is the original 14-frame expression sheet.
var idle = []frame{
	f(1400, eyeOpen, eyeOpen, mouthSmile),
	f(55, eyeHalf, eyeOpen, mouthSmile),
	f(85, eyeClosed, eyeClosed, mouthSmile),
	f(65, eyeOpen, eyeHalf, mouthSmile),
	f(1100, eyeOpen, eyeOpen, mouthSmile),
	f(420, eyeLeft, eyeLeft, mouthSmall),
	f(420, eyeRight, eyeRight, mouthSmall),
	f(240, eyeWide, eyeWide, mouthSmall),
	f(450, eyeHappy, eyeHappy, mouthSmall),
	f(180, eyeWide, eyeWide, mouthOpen),
	f(115, eyeHappy, eyeHappy, mouthChew),
	f(200, eyeClosed, eyeClosed, mouthSmall),
	f(650, eyeHappy, eyeHappy, mouthSmile),
	f(130, eyeOpen, eyeClosed, mouthSmile),
}

// Moods are short reactions; Nox returns to idle afterwards.
var (
	moodLookLeft  = pose(eyeLeft, eyeLeft, mouthSmall)
	moodLookRight = pose(eyeRight, eyeRight, mouthSmall)
	moodHappy     = pose(eyeHappy, eyeHappy, mouthSmile)
	moodSurprised = pose(eyeWide, eyeWide, mouthOpen)
	moodSleep     = pose(eyeClosed, eyeClosed, mouthSmall)
	moodStill     = pose(eyeOpen, eyeOpen, mouthSmile)
)

// Ambient life — the portfolio's own scheduler (J-Portfolio
// src/components/game/HiddenResident.tsx + src/assets/residentAnimation.ts):
// Nox rests 2.2–4.6 s, then blinks; after a quiet spell without input it
// searches for you (≥6.5 s) or dozes off (≥22 s), at most one such cue every
// 14 s. Asleep it stays asleep until a key wakes it. Durations are the exported
// per-frame ones (art/resident/sheet.json); faces are the block sprites above.

// gesture is a short run of faces played once, then Nox rests again.
type gesture struct {
	steps  []frame
	sleeps bool // ends asleep (eyes stay shut until a key)
}

func (g gesture) length() time.Duration {
	var d time.Duration
	for _, s := range g.steps {
		d += s.d
	}
	return d
}

// blink is residentBlink: a frog-like stagger, the left lid leads and the right
// one starts before the left has opened — never both fully shut together.
var blink = gesture{steps: []frame{
	f(65, eyeHalf, eyeOpen, mouthSmile),
	f(85, eyeClosed, eyeOpen, mouthSmile),
	f(55, eyeClosed, eyeHalf, mouthSmile),
	f(75, eyeHalf, eyeClosed, mouthSmile),
	f(115, eyeOpen, eyeClosed, mouthSmile),
	f(85, eyeOpen, eyeHalf, mouthSmile),
	f(80, eyeOpen, eyeOpen, mouthSmile),
}}

// notice (clip "notice", frames 72–75): played once when the list opens.
var notice = gesture{steps: []frame{
	f(350, eyeWide, eyeWide, mouthSmall),
	f(110, eyeOpen, eyeOpen, mouthSmall),
	f(220, eyeHappy, eyeHappy, mouthSmile),
	f(450, eyeOpen, eyeOpen, mouthSmile),
}}

// search (frames 76–80): looks for you after a quiet spell.
var search = gesture{steps: []frame{
	f(600, eyeLeft, eyeLeft, mouthSmall),
	f(130, eyeOpen, eyeOpen, mouthSmall),
	f(700, eyeRight, eyeRight, mouthSmall),
	f(130, eyeOpen, eyeOpen, mouthSmall),
	f(400, eyeOpen, eyeOpen, mouthSmile),
}}

// sleepy (frames 61–65): droops, fights it once, then sleeps and holds.
var sleepy = gesture{sleeps: true, steps: []frame{
	f(700, eyeHalf, eyeHalf, mouthSmile),
	f(220, eyeOpen, eyeOpen, mouthSmile),
	f(900, eyeHalf, eyeHalf, mouthSmall),
	f(200, eyeClosed, eyeHalf, mouthSmall),
	f(1200, eyeClosed, eyeClosed, mouthSmall),
}}

// asleep is the face held until a key.
var asleep = pose(eyeClosed, eyeClosed, mouthSmall)

// wake (frames 68–71, "notice" after sleep): a key wakes Nox up.
var wake = gesture{steps: []frame{
	f(150, eyeHalf, eyeHalf, mouthSmall),
	f(220, eyeWide, eyeWide, mouthSmall),
	f(180, eyeOpen, eyeOpen, mouthSmall),
	f(500, eyeOpen, eyeOpen, mouthSmile),
}}

// surprised (frames 20–25): errors and "delete for good?".
var surprised = gesture{steps: []frame{
	f(140, eyeWide, eyeWide, mouthSmall),
	f(100, eyeWide, eyeWide, mouthOpen),
	f(420, eyeWide, eyeWide, mouthOpen),
	f(160, eyeWide, eyeWide, mouthSmall),
	f(180, eyeOpen, eyeOpen, mouthSmall),
	f(350, eyeOpen, eyeOpen, mouthSmile),
}}

// idleWait is the rest before the next blink or cue: 2200 + rand·2400 ms.
func idleWait() time.Duration {
	return 2200*time.Millisecond + time.Duration(rand.Int63n(int64(2400*time.Millisecond)))
}

// Thresholds from residentIdleCue.
const (
	cueCooldown = 14 * time.Second
	searchAfter = 6500 * time.Millisecond
	sleepAfter  = 22 * time.Second
)

// nextGesture is residentIdleCue: quiet = time since the last key, cool = time
// since the last cue. It reports whether a cue (not a plain blink) was chosen.
func nextGesture(quiet, cool time.Duration) (gesture, bool) {
	switch {
	case cool < cueCooldown:
		return blink, false
	case quiet >= sleepAfter:
		return sleepy, true
	case quiet >= searchAfter:
		return search, true
	}
	return blink, false
}
