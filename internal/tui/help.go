package tui

import (
	"strings"
)

// helpOrder is the order of the help groups after the current tab's.
var helpOrder = []string{grToday, grWeek, grMonth, grInbox, grFocus, grRules, grGo, grApp}

// helpBody is the help card: every keyed action by group, the current
// tab's group first, in two columns when the screen is wide enough.
func (m *Model) helpBody(w int) []string {
	first := tabGroup[m.tab]
	if m.tab == tabWeek && m.week.month {
		first = grMonth
	}
	order := []string{first}
	for _, g := range helpOrder {
		if g != first {
			order = append(order, g)
		}
	}
	var blocks [][]string
	for _, g := range order {
		lines := []string{sHead.Render(g)}
		seen := map[string]bool{}
		if g == grGo {
			lines = append(lines, "  "+sBrand.Bold(true).Render(fit("1-5", 6))+" "+sText.Render("switch tab"))
			seen["tab"+"next tab"] = false
		}
		for _, a := range actionTable() {
			if g == grGo && a.key != "tab" {
				continue
			}
			if a.group != g || a.key == "" || seen[a.key+a.name] {
				continue
			}
			seen[a.key+a.name] = true
			lines = append(lines, "  "+sBrand.Bold(true).Render(fit(a.key, 6))+" "+sText.Render(a.name))
		}
		if g == grApp {
			lines = append(lines, "  "+sBrand.Bold(true).Render(fit("ctrl+k", 6))+" "+sText.Render("commands too"))
		}
		blocks = append(blocks, append(lines, ""))
	}
	cw := min(w-2, 120)
	inner := cw - 4
	cols := 1
	for c := 3; c > 1; c-- {
		if (inner-3*(c-1))/c >= 26 {
			cols = c
			break
		}
	}
	colW := (inner - 3*(cols-1)) / cols
	// Fill columns in order, each up to an even share of the lines.
	total := 0
	for _, b := range blocks {
		total += len(b)
	}
	share := (total + cols - 1) / cols
	columns := make([][]string, cols)
	_ = share
	for _, b := range blocks {
		c := 0
		for i := range columns {
			if len(columns[i]) < len(columns[c]) {
				c = i
			}
		}
		columns[c] = append(columns[c], b...)
	}
	body := columns[0]
	for i, col := range columns[1:] {
		body = sideBySide(body, col, (colW+3)*(i+1)-3, 3)
	}
	legend := strings.Join([]string{dot("deep") + " deep", dot("shallow") + " shallow", dot("neutral") + " neutral",
		dot("browse") + " browsing", dot("distract") + " distraction", dot("") + " unlabelled"}, "   ")
	body = append(body, legend)
	return card("Keys", body, cw)
}

// ansiWidthMax is the widest line of lines.
func ansiWidthMax(lines []string) int {
	w := 0
	for _, l := range lines {
		w = max(w, ansiWidth(l))
	}
	return w
}
