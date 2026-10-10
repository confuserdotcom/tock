package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tock/internal/classify"
)

// Noir: grays and one accent for distraction and alerts.
var (
	colText   = lipgloss.Color("#c8c8c8")
	colBright = lipgloss.Color("#f0f0f0")
	colDim    = lipgloss.Color("#8a8a8a")
	colFaint  = lipgloss.Color("#4e4e4e")
	colAccent = lipgloss.Color("#a45f5f")

	sText   = lipgloss.NewStyle().Foreground(colText)
	sBright = lipgloss.NewStyle().Foreground(colBright)
	sHead   = lipgloss.NewStyle().Foreground(colBright).Bold(true)
	sDim    = lipgloss.NewStyle().Foreground(colDim)
	sFaint  = lipgloss.NewStyle().Foreground(colFaint)
	sAccent = lipgloss.NewStyle().Foreground(colAccent)
	sSel    = lipgloss.NewStyle().Reverse(true)
)

// kindStyle colours time by category kind; "" is unlabelled.
func kindStyle(kind string) lipgloss.Style {
	switch kind {
	case classify.KindDeep:
		return sBright
	case classify.KindShallow:
		return sDim
	case classify.KindNeutral:
		return sFaint
	case classify.KindDistract, "":
		return sAccent
	}
	return sText
}

// kindName is the short row name of a kind in the heatmap and stats.
func kindName(kind string) string {
	if kind == "" {
		return "unlabelled"
	}
	return kind
}

// fit truncates s to w display cells (with …) and pads it to exactly w.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-ansi.StringWidth(s))
}

// fitRight right-aligns s in w cells.
func fitRight(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return fit(s, w)
}

// spread puts left and right on one line of width w.
func spread(left, right string, w int) string {
	gap := w - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return fit(left, w)
	}
	return left + strings.Repeat(" ", gap) + right
}
