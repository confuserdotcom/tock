package tui

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	switch {
	case m.menu != nil:
		return m.menuKey(k)
	case m.palette != nil:
		return m.paletteKey(msg)
	case m.picker != nil:
		return m.pickerKey(msg)
	case m.start != nil:
		return m.startKey(msg)
	case m.edit != nil:
		return m.rangeKey(msg)
	case m.ask != nil:
		return m.askKey(msg)
	case m.choose != nil:
		return m.chooseKey(k)
	case m.learn != nil:
		return m.learnKey(k)
	case m.help:
		switch k {
		case "?", "esc", "q":
			m.help = false
		case ":", "ctrl+k":
			m.help, m.palette = false, &palette{}
		}
		return m, nil
	case m.score:
		switch k {
		case "?":
			m.score, m.help = false, true
		case "esc", "q", "i":
			m.score = false
		}
		return m, nil
	case m.tab == tabToday && m.today.filtering:
		m.today.filterKey(msg)
		return m, nil
	case m.tab == tabPlan && m.plan.adding:
		return m.planAddKey(msg)
	}
	switch k {
	case "q":
		return m, tea.Quit
	case "?":
		m.help = true
		return m, nil
	case ":", "ctrl+k":
		m.palette = &palette{}
		return m, nil
	case "i":
		if m.tab == tabToday {
			m.score = true
			return m, nil
		}
	case "r":
		m.plan.force = true
		return m, m.reload()
	case "u", "ctrl+z":
		if m.tab == tabPlan && m.plan.last != nil {
			return m, m.planUndoLast()
		}
		return m, m.undo()
	case "1", "2", "3", "4", "5", "6", "7":
		return m.switchTab(int(k[0] - '1'))
	case "tab":
		return m.switchTab(m.nextTab(1))
	case "shift+tab":
		return m.switchTab(m.nextTab(numTabs - 1))
	}
	switch m.tab {
	case tabToday:
		return m.todayKey(k)
	case tabWeek:
		return m.weekKey(k)
	case tabFocus:
		return m.focusKey(k)
	case tabRules:
		return m.rulesKey(k)
	case tabPlan:
		return m.planKey(k)
	case tabLabels:
		return m.labelsKey(k)
	}
	return m.inboxKey(k)
}

// nextTab is the shown tab step tabs after the open one.
func (m Model) nextTab(step int) int {
	t := (m.tab + step) % numTabs
	if t == tabFocus && m.noFocus {
		t = (t + step) % numTabs
	}
	return t
}

func (m Model) switchTab(tab int) (tea.Model, tea.Cmd) {
	if tab == tabFocus && m.noFocus {
		m.setFlash("Focus is off · tock choice focus on", false)
		return m, nil
	}
	if tab == m.tab {
		return m, nil
	}
	m.tab = tab
	if tab == tabPlan {
		m.plan.force = true
	}
	return m, m.reload()
}
