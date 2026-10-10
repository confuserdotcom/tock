package tui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// palette is the command palette: every available action, filtered by
// what is typed.
type palette struct {
	query  string
	cursor int
}

// paletteMax is how many matches the palette lists.
const paletteMax = 12

// match scores how well q matches s as a subsequence, case-insensitively:
// -1 for no match; higher is better (a prefix, then contiguous runs).
func match(q, s string) int {
	q, s = strings.ToLower(q), strings.ToLower(s)
	if q == "" {
		return 0
	}
	if strings.HasPrefix(s, q) {
		return 1000 - len(s)
	}
	if i := strings.Index(s, q); i >= 0 {
		return 500 - i
	}
	score, at, run := 0, 0, 0
	for _, c := range q {
		i := strings.IndexRune(s[at:], c)
		if i < 0 {
			return -1
		}
		if i == 0 {
			run++
			score += run
		} else {
			run = 0
		}
		at += i + len(string(c))
	}
	return score
}

// items is the available actions matching the query, best first; the
// current tab's group wins ties.
func (p *palette) items(m *Model) []action {
	type scored struct {
		a     action
		score int
		i     int
	}
	var list []scored
	for i, a := range m.available() {
		if a.key == ":" {
			continue
		}
		sc := max(match(p.query, a.name), match(p.query, a.group+" "+a.name))
		if sc < 0 {
			continue
		}
		if a.group == tabGroup[m.tab] || (m.tab == tabWeek && m.week.month && a.group == grMonth) {
			sc += 50
		}
		list = append(list, scored{a, sc, i})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].score > list[j].score })
	out := make([]action, 0, len(list))
	for _, s := range list {
		out = append(out, s.a)
	}
	return out
}

func (m Model) paletteKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.palette
	items := p.items(&m)
	switch k := msg.String(); k {
	case "esc":
		m.palette = nil
	case "up", "ctrl+p":
		p.cursor = max(p.cursor-1, 0)
	case "down", "ctrl+n", "tab":
		p.cursor = min(p.cursor+1, max(min(len(items), paletteMax)-1, 0))
	case "enter":
		if p.cursor < len(items) {
			m.palette = nil
			return m.runAction(items[p.cursor])
		}
	case "backspace":
		if r := []rune(p.query); len(r) > 0 {
			p.query, p.cursor = string(r[:len(r)-1]), 0
		}
	default:
		if msg.Text != "" {
			p.query, p.cursor = p.query+msg.Text, 0
		}
	}
	return m, nil
}

// view is the palette card, w cells wide at most.
func (p *palette) view(m *Model, w int) []string {
	cw := min(w-4, 64)
	inner := cw - 4
	lines := []string{sBrand.Render("›") + " " + sBright.Render(p.query) + sBrand.Render("▏"), ""}
	items := p.items(m)
	if len(items) == 0 {
		lines = append(lines, sDim.Render("no command matches"))
	}
	for i, a := range items[:min(len(items), paletteMax)] {
		k := a.key
		right := sDim.Render(a.group)
		if k != "" {
			right += "  " + sBrand.Bold(true).Render(k)
		}
		left := a.name
		gap := inner - 2 - ansi.StringWidth(left) - ansi.StringWidth(right)
		line := left + strings.Repeat(" ", max(gap, 1)) + right
		if i == p.cursor {
			lines = append(lines, selRow(sBrand.Render("▸ ")+sBright.Render(line), inner))
		} else {
			lines = append(lines, "  "+line)
		}
	}
	return card("Commands", lines, cw)
}
