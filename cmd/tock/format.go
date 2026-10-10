package main

import (
	"fmt"
	"strings"
)

// fmtDur renders milliseconds as "2h14", "41m" or "45s".
func fmtDur(ms int64) string {
	s := ms / 1000
	switch {
	case s >= 3600:
		return fmt.Sprintf("%dh%02d", s/3600, s%3600/60)
	case s >= 60:
		return fmt.Sprintf("%dm", s/60)
	}
	return fmt.Sprintf("%ds", s)
}

const barCells = 12

// bar renders ms relative to max as up to 12 cells of █ plus a half cell ▌.
// Any non-zero time shows at least a half cell.
func bar(ms, max int64) string {
	if ms <= 0 || max <= 0 {
		return ""
	}
	halves := ms * barCells * 2 / max
	if halves == 0 {
		halves = 1
	}
	return strings.Repeat("█", int(halves/2)) + strings.Repeat("▌", int(halves%2))
}

// pad right-pads s with spaces to n runes.
func pad(s string, n int) string {
	if k := n - len([]rune(s)); k > 0 {
		return s + strings.Repeat(" ", k)
	}
	return s
}

// safe replaces control characters so window titles cannot drive the
// terminal.
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return '?'
		}
		return r
	}, s)
}

// labelText renders a category/project pair as "Study / ATLAS", "Study" or
// "project ATLAS".
func labelText(category, project string) string {
	switch {
	case category != "" && project != "":
		return category + " / " + project
	case category != "":
		return category
	}
	return "project " + project
}
