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
	// DESIGN.md: paper by day, warm ink at night, brass for the one thing to look at.
	colText = hex("#34343a", "#ddd6c8")
	colBright = hex("#1d1c19", "#f4ecdd")
	colDim = hex("#5f636c", "#8d93a0")
	colFaint = hex("#9a9ea6", "#5b606b")
	colSurface = hex("#e6e6e0", "#2a2822")
	colHover = hex("#f0f0ec", "#1a1915")
	colBrand = hex("#94661b", "#e3a446")
	colDeep = hex("#2d7a68", "#63bfa8")
	colShallow = hex("#5a78a3", "#8fa9d1")
	colNeutral = hex("#858991", "#7d828c")
	colAccent = hex("#b5462c", "#e57d62")
	colBrowse = hex("#9a6585", "#c993b4")
	// Unlabelled: the web hatches grey; a terminal cannot, so a lighter grey.
	colUnlabelled = hex("#b0aea6", "#56554f")
	colGood = hex("#2d7a68", "#63bfa8")

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
