package report

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// MaxNoteRunes is the longest session note, in runes.
const MaxNoteRunes = 200

// CleanNote makes s a safe one-line note: line breaks and tabs become
// spaces, runs of spaces collapse, edges are trimmed and control characters
// become '?'. runes is the length after cleaning and before the cut to
// MaxNoteRunes, so a caller can refuse an over-long note instead of
// silently truncating it.
func CleanNote(s string) (note string, runes int) {
	s = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)
	s = Safe(strings.Join(strings.Fields(s), " "))
	r := []rune(s)
	runes = len(r)
	if runes > MaxNoteRunes {
		r = r[:MaxNoteRunes]
	}
	return strings.TrimRight(string(r), " "), runes
}

// Cut shortens s to at most cells terminal cells, ending in "…" when it was
// cut. It never splits a rune.
func Cut(s string, cells int) string {
	return ansi.Truncate(s, cells, "…")
}
