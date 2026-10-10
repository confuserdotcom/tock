// Package report turns labelled spans into the numbers and rows that tock
// prints and draws. It is pure: no database, no terminal, no clock.
package report

import (
	"fmt"
	"strings"
)

// Dur renders milliseconds as "2h14", "41m" or "45s".
func Dur(ms int64) string {
	s := ms / 1000
	switch {
	case s >= 3600:
		return fmt.Sprintf("%dh%02d", s/3600, s%3600/60)
	case s >= 60:
		return fmt.Sprintf("%dm", s/60)
	}
	return fmt.Sprintf("%ds", s)
}

// Bar renders ms relative to max as up to cells full blocks plus a half
// block ▌. Any non-zero time shows at least a half block.
func Bar(ms, max int64, cells int) string {
	if ms <= 0 || max <= 0 || cells <= 0 {
		return ""
	}
	halves := ms * int64(cells) * 2 / max
	if halves == 0 {
		halves = 1
	}
	return strings.Repeat("█", int(halves/2)) + strings.Repeat("▌", int(halves%2))
}

// Pad right-pads s with spaces to n runes.
func Pad(s string, n int) string {
	if k := n - len([]rune(s)); k > 0 {
		return s + strings.Repeat(" ", k)
	}
	return s
}

// Safe replaces control characters so window titles cannot drive the
// terminal.
func Safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return '?'
		}
		return r
	}, s)
}

// LabelText renders a category/project pair as "Study / ATLAS", "Study" or
// "project ATLAS".
func LabelText(category, project string) string {
	switch {
	case category != "" && project != "":
		return category + " / " + project
	case category != "":
		return category
	}
	return "project " + project
}
