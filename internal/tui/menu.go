package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// ctxMenu is the right-click menu: the open tab's actions for the selected
// row, drawn where the mouse was.
type ctxMenu struct {
	x, y   int
	items  []action
	cursor int
}

// newMenu builds the menu of the open tab's actions at (x, y), or nil when
// there are none.
func newMenu(m *Model, x, y int) *ctxMenu {
	var items []action
	for _, a := range m.available() {
		if a.prio > 0 && a.group == tabGroup[m.tab] && a.key != "" {
			items = append(items, a)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &ctxMenu{x: x, y: y, items: items}
}

func menuItem(id string) (int, bool) {
	kind, arg, _ := strings.Cut(id, ":")
	if kind != "menu" {
		return 0, false
	}
	i, err := strconv.Atoi(arg)
	return i, err == nil
}

// runMenu closes the menu and runs item i.
func (m Model) runMenu(i int) (tea.Model, tea.Cmd) {
	mn := m.menu
	m.menu = nil
	if mn == nil || i < 0 || i >= len(mn.items) {
		return m, nil
	}
	return m.runAction(mn.items[i])
}

// menuKey handles keys while the menu is open: arrows move, enter runs, an
// item's key runs it, anything else closes the menu.
func (m Model) menuKey(k string) (tea.Model, tea.Cmd) {
	mn := m.menu
	switch k {
	case "up", "k":
		mn.cursor = max(mn.cursor-1, 0)
		return m, nil
	case "down", "j":
		mn.cursor = min(mn.cursor+1, len(mn.items)-1)
		return m, nil
	case "enter":
		return m.runMenu(mn.cursor)
	}
	for i, a := range mn.items {
		if a.key == k {
			return m.runMenu(i)
		}
	}
	m.menu = nil
	return m, nil
}

// lines is the menu's card.
func (mn *ctxMenu) lines() []string {
	kw, nw := 1, 1
	for _, a := range mn.items {
		kw = max(kw, ansi.StringWidth(a.key))
		nw = max(nw, ansi.StringWidth(a.name))
	}
	var body []string
	for i, a := range mn.items {
		row := " " + sBrand.Bold(true).Render(fit(a.key, kw)) + "  " + sText.Render(fit(a.name, nw)) + " "
		if i == mn.cursor {
			row = selRow(row, kw+nw+4)
		}
		body = append(body, z("menu:"+strconv.Itoa(i), row))
	}
	return card("", body, kw+nw+8)
}

// paintAt draws block over lines with its top-left cell at (x, y), clipped
// to the screen, leaving the rest of each line as it was.
func paintAt(lines, block []string, x, y, w int) []string {
	bw := 0
	for _, l := range block {
		bw = max(bw, ansi.StringWidth(l))
	}
	x = max(min(x, w-bw), 0)
	y = max(min(y, len(lines)-len(block)), 0)
	out := append([]string{}, lines...)
	for i, l := range block {
		row := y + i
		if row >= len(out) {
			break
		}
		base := out[row] + strings.Repeat(" ", max(w-ansi.StringWidth(out[row]), 0))
		out[row] = ansi.Cut(base, 0, x) + "\x1b[m" + l + "\x1b[m" + ansi.Cut(base, x+ansi.StringWidth(l), w)
	}
	return out
}
