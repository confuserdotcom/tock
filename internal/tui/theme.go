package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"tock/internal/classify"
)

// The palette: soft colors, one per kind of time, with a dark and a light
// value each. setTheme picks one set; the TUI calls it when the terminal
// reports its background.
var (
	colText, colBright, colDim, colFaint, colSurface color.Color
	colBrand, colGood, colAccent                     color.Color
	colDeep, colShallow, colNeutral, colUnlabelled   color.Color
	colBrowse                                        color.Color

	sText, sBright, sHead, sDim, sFaint, sAccent, sSel lipgloss.Style
	sBrand, sGood, sUnl                                lipgloss.Style
	sDeep, sShallow, sNeutral, sBrowse                 lipgloss.Style

	themeDark = true
)

func init() { setTheme(true) }

// setTheme builds every style for a dark or a light terminal.
func setTheme(dark bool) {
	themeDark = dark
	ld := lipgloss.LightDark(dark)
	hex := func(light, darkHex string) color.Color { return ld(lipgloss.Color(light), lipgloss.Color(darkHex)) }
	colText = hex("#2b2d33", "#d8dae0")
	colBright = hex("#111216", "#f4f5f7")
	colDim = hex("#6b7080", "#8b90a0")
	colFaint = hex("#b4b8c4", "#4a4f5c")
	colSurface = hex("#e9ebf0", "#252833")
	colBrand = hex("#4d5ae0", "#9aa5ff")
	colDeep = hex("#1f8a73", "#5fc4ad")
	colShallow = hex("#a07818", "#e0bf72")
	colNeutral = hex("#6b7487", "#8a94a6")
	colAccent = hex("#c4503a", "#ef8a76")
	colBrowse = hex("#b8692a", "#eba06a")
	colUnlabelled = hex("#7652c9", "#b69cf0")
	colGood = hex("#3c8a36", "#8fd18a")

	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	sText, sBright, sDim, sFaint = fg(colText), fg(colBright), fg(colDim), fg(colFaint)
	sHead = fg(colBright).Bold(true)
	sAccent, sBrand, sGood, sUnl = fg(colAccent), fg(colBrand), fg(colGood), fg(colUnlabelled)
	sDeep, sShallow, sNeutral, sBrowse = fg(colDeep), fg(colShallow), fg(colNeutral), fg(colBrowse)
	sSel = lipgloss.NewStyle().Background(colSurface).Foreground(colBright)
}

// kindStyle colours time by category kind; "" is unlabelled.
func kindStyle(kind string) lipgloss.Style {
	switch kind {
	case classify.KindDeep:
		return sDeep
	case classify.KindShallow:
		return sShallow
	case classify.KindNeutral:
		return sNeutral
	case classify.KindBrowse:
		return sBrowse
	case classify.KindDistract:
		return sAccent
	case "":
		return sUnl
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
