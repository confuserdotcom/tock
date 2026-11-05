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
	colBrowse, colHover                              color.Color

	sText, sBright, sHead, sDim, sFaint, sAccent, sSel lipgloss.Style
	sHover                                             lipgloss.Style
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
	// DESIGN.md: the ledger. Greys, ink as the only accent, colour for kinds of time.
	colText = hex("#2b3038", "#d5d8dd")
	colBright = hex("#1b1f24", "#eceef1")
	colDim = hex("#5b636e", "#9aa1ab")
	colFaint = hex("#6b7280", "#868d97")
	colSurface = hex("#f1f1f3", "#1d2024")
	colHover = hex("#f7f7f8", "#16181b")
	colBrand = hex("#1b1f24", "#eceef1")
	colDeep = hex("#2f6f8f", "#5fa3c4")
	colShallow = hex("#7d8fa8", "#8fa1ba")
	colNeutral = hex("#a3a8b0", "#6b7079")
	colAccent = hex("#c0504d", "#e0726f")
	colBrowse = hex("#8a6d9e", "#b39cc6")
	// Unlabelled: the web hatches grey; a terminal cannot, so a lighter grey.
	colUnlabelled = hex("#c4c7cc", "#4a4e55")
	colGood = hex("#2e7d4f", "#5cc38a")

	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
	sText, sBright, sDim, sFaint = fg(colText), fg(colBright), fg(colDim), fg(colFaint)
	sHead = fg(colBright).Bold(true)
	sAccent, sBrand, sGood, sUnl = fg(colAccent), fg(colBrand), fg(colGood), fg(colUnlabelled)
	sDeep, sShallow, sNeutral, sBrowse = fg(colDeep), fg(colShallow), fg(colNeutral), fg(colBrowse)
	sSel = lipgloss.NewStyle().Background(colSurface).Foreground(colBright)
	sHover = lipgloss.NewStyle().Background(colHover)
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
