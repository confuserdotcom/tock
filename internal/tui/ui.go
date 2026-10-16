package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// card draws lines in a rounded border w cells wide, with title set into the
// top edge. Lines are fitted to the inside width.
func card(title string, lines []string, w int) []string {
	inner := max(w-4, 1)
	edge := func(n int) string { return strings.Repeat("─", max(n, 0)) }
	top := sFaint.Render("╭" + edge(w-2) + "╮")
	if title != "" {
		t := ansi.Truncate(title, max(w-6, 1), "…")
		top = sFaint.Render("╭─ ") + sDim.Render(t) + sFaint.Render(" "+edge(w-5-ansi.StringWidth(t))+"╮")
	}
	out := []string{top}
	side := sFaint.Render("│")
	for _, l := range lines {
		out = append(out, side+" "+fit(l, inner)+" "+side)
	}
	return append(out, sFaint.Render("╰"+edge(w-2)+"╯"))
}

// sideBySide puts b to the right of a, which is aw cells wide, gap cells
// apart. The shorter column is padded with blank lines.
func sideBySide(a, b []string, aw, gap int) []string {
	n := max(len(a), len(b))
	out := make([]string, n)
	for i := range n {
		var l, r string
		if i < len(a) {
			l = a[i]
		}
		if i < len(b) {
			r = b[i]
		}
		out[i] = fit(l, aw) + strings.Repeat(" ", gap) + r
	}
	return out
}

// meter is a bar w cells wide: v/total of it in st, the rest faint.
func meter(v, total int64, w int, st lipgloss.Style) string {
	if w <= 0 {
		return ""
	}
	n := 0
	if total > 0 {
		n = int(min(max((v*int64(w)+total/2)/total, 0), int64(w)))
	}
	if v > 0 && n == 0 {
		n = 1
	}
	return st.Render(strings.Repeat("━", n)) + sFaint.Render(strings.Repeat("─", w-n))
}

// dot is a kind's marker: ● in its colour, ○ for unlabelled.
func dot(kind string) string {
	if kind == "" {
		return sUnl.Render("○")
	}
	return kindStyle(kind).Render("●")
}

// keycap renders a key and what it does: "t tag". It is a button: a click
// presses the key. Pairs like "j/k" only describe keys.
func keycap(k, label string) string {
	s := sBrand.Bold(true).Render(k) + " " + sDim.Render(label)
	if strings.Contains(k, "/") && k != "/" {
		return s
	}
	return z("key:"+k, s)
}

// pill renders text on a coloured background.
func pill(text string, st lipgloss.Style) string {
	return st.Reverse(true).Bold(true).Render(" " + text + " ")
}

// scoreBand is a score's colour and the word for it.
func scoreBand(score int, active bool) (lipgloss.Style, string) {
	switch {
	case !active:
		return sDim, "not started"
	case score >= 80:
		return sGood, "great day"
	case score >= 60:
		return sDeep, "good day"
	case score >= 40:
		return sShallow, "steady"
	}
	return sAccent, "scattered"
}

// glyphs is a 3-row block font for the focus countdown.
var glyphs = map[rune][3]string{
	'0': {"█▀█", "█ █", "▀▀▀"},
	'1': {"▀█ ", " █ ", "▀▀▀"},
	'2': {"▀▀█", "█▀▀", "▀▀▀"},
	'3': {"▀▀█", " ▀█", "▀▀▀"},
	'4': {"█ █", "▀▀█", "  ▀"},
	'5': {"█▀▀", "▀▀█", "▀▀▀"},
	'6': {"█▀▀", "█▀█", "▀▀▀"},
	'7': {"▀▀█", "  █", "  ▀"},
	'8': {"█▀█", "█▀█", "▀▀▀"},
	'9': {"█▀█", "▀▀█", "▀▀▀"},
	':': {" ", "▀", "▀"},
	'-': {"   ", "▀▀▀", "   "},
}

// bigDigits renders s (digits, ':' and '-') in the block font, glyphs one
// cell apart. Other runes are skipped.
func bigDigits(s string) [3]string {
	var rows [3][]string
	for _, r := range s {
		g, ok := glyphs[r]
		if !ok {
			continue
		}
		for i := range rows {
			rows[i] = append(rows[i], g[i])
		}
	}
	var out [3]string
	for i := range rows {
		out[i] = strings.Join(rows[i], " ")
	}
	return out
}

// center pads s on the left to centre it in w cells.
func center(s string, w int) string {
	return strings.Repeat(" ", max((w-ansi.StringWidth(s))/2, 0)) + s
}

// emptyState is a centred card with a message, for empty tabs.
func emptyState(w int, title string, lines ...string) []string {
	cw := min(max(w-8, 30), 64)
	pad := strings.Repeat(" ", max((w-cw)/2, 0))
	body := append([]string{""}, lines...)
	body = append(body, "")
	out := []string{""}
	for _, l := range card(title, body, cw) {
		out = append(out, pad+l)
	}
	return out
}

func ansiWidth(s string) int { return ansi.StringWidth(s) }

// overlay draws top over base, both lines of a w-wide screen: where a top
// line has content, that content replaces the cells it covers; everything
// else of base shows through, faded, so the tab stays visible behind a
// dialog.
func overlay(base, top []string, w int) []string {
	out := make([]string, len(base))
	for i := range base {
		under := ansi.Strip(base[i])
		var t string
		if i < len(top) {
			t = top[i]
		}
		plain := strings.TrimRight(ansi.Strip(t), " ")
		if strings.TrimSpace(plain) == "" {
			out[i] = sFaint.Render(under)
			continue
		}
		x := len(plain) - len(strings.TrimLeft(plain, " "))
		end := ansi.StringWidth(plain)
		under += strings.Repeat(" ", max(w-ansi.StringWidth(under), 0))
		out[i] = sFaint.Render(ansi.Cut(under, 0, x)) + ansi.Cut(t, x, end) + sFaint.Render(ansi.Cut(under, end, w))
	}
	return out
}

// selRow paints a selected row w cells wide on the surface colour while
// keeping its own colours: the background is set again after every reset
// in s.
func selRow(s string, w int) string {
	probe := sSel.Render("x")
	bg, _, _ := strings.Cut(probe, "x")
	if bg == "" { // no colour (tests strip it anyway)
		return fit(s, w)
	}
	s = fit(s, w)
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+bg)
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+bg)
	return bg + s + "\x1b[m"
}

// navArrows are the "‹ h  l ›" buttons beside a day, week or month title.
func navArrows() string {
	return sFaint.Render("  ") + z("key:h", sFaint.Render("‹ h")) + sFaint.Render("  ") + z("key:l", sFaint.Render("l ›"))
}
