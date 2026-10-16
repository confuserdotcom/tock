package tui

import (
	"regexp"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Mouse input is turned into the key presses or cursor moves the keyboard
// would make, so behaviour has one code path.

// keyPress is the key press for a footer or dialog key name.
func keyPress(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

// overlay reports whether a dialog or full-body page covers the tab.
func (m Model) overlay() bool {
	return m.picker != nil || m.learn != nil || m.start != nil || m.edit != nil || m.help || m.score ||
		(m.tab == tabPlan && m.plan.adding)
}

func (m Model) tooSmall() bool { return m.width < MinWidth || m.height < MinHeight }

// filtering reports whether the today filter is being typed.
func (m Model) filtering() bool { return m.tab == tabToday && m.today.filtering }

// wheel scrolls by moving the cursor, as up and down do.
func (m Model) wheel(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if m.tooSmall() || m.filtering() || m.help || m.score {
		return m, nil
	}
	switch ms.Button {
	case tea.MouseWheelUp:
		return m.key(keyPress("up"))
	case tea.MouseWheelDown:
		return m.key(keyPress("down"))
	}
	return m, nil
}

// bodyTop is the screen row of the body's first line.
func (m Model) bodyTop() int {
	if m.banner() != "" {
		return 2
	}
	return 1
}

func (m Model) click(ms tea.Mouse) (tea.Model, tea.Cmd) {
	if m.tooSmall() || ms.Button != tea.MouseLeft {
		return m, nil
	}
	switch y := ms.Y; {
	case y == 0:
		if t, ok := m.tabAt(ms.X); ok && !m.overlay() && !m.filtering() {
			return m.switchTab(t)
		}
	case y == m.height-1:
		if k, ok := footerKey(m.footerText(), ms.X); ok {
			return m.key(keyPress(k))
		}
	case y >= m.bodyTop() && !m.filtering():
		return m.clickBody(y-m.bodyTop(), ms.X)
	}
	return m, nil
}

// tabAt is the tab whose name in the header covers column x.
func (m Model) tabAt(x int) (int, bool) {
	at := ansi.StringWidth("tock") + len(headerGap)
	for i, n := range m.tabNames() {
		w := ansi.StringWidth(n)
		if x >= at && x < at+w {
			return i, true
		}
		at += w + len(headerGap)
	}
	return 0, false
}

// footerText is the footer's hint, or "" while a flash replaces it.
func (m Model) footerText() string {
	if m.flash != "" {
		return ""
	}
	return ansi.Strip(fit(m.hint(), m.width))
}

// footerKey is the key of the hint segment covering column x. A segment is a
// button when its first word is a key: one character, enter or esc.
func footerKey(hint string, x int) (string, bool) {
	const sep = " · "
	at := 0
	for _, seg := range strings.Split(hint, sep) {
		w := ansi.StringWidth(seg)
		if x >= at && x < at+w {
			k, _, _ := strings.Cut(strings.TrimSpace(seg), " ")
			if k == "enter" || k == "esc" || utf8.RuneCountInString(k) == 1 {
				return k, true
			}
			return "", false
		}
		at += w + ansi.StringWidth(sep)
	}
	return "", false
}

// clickBody handles a click on body line line. Clicking a row selects it;
// clicking the selected row again does what enter does, where enter does
// something.
func (m Model) clickBody(line, x int) (tea.Model, tea.Cmd) {
	h := m.bodyH()
	switch {
	case m.picker != nil:
		if i, ok := m.picker.rowAt(line, m.width, h); ok {
			if i == m.picker.cursor {
				return m.key(keyPress("enter"))
			}
			m.picker.cursor = i
		}
	case m.start != nil:
		if i, ok := m.start.rowAt(line); ok {
			if i == m.start.cursor {
				return m.key(keyPress("enter"))
			}
			m.start.cursor = i
		}
	case m.learn != nil:
		body := m.body(h)
		if line < len(body) {
			if k, ok := buttonAt(ansi.Strip(body[line]), x); ok {
				return m.key(keyPress(k))
			}
		}
	case m.overlay():
	case m.tab == tabToday:
		if i, ok := m.today.rowAt(line, m.width, h); ok {
			m.today.move(i - m.today.cursor)
		}
	case m.tab == tabWeek:
		if i, ok := m.week.rowAt(line); ok {
			if i == m.week.cursor {
				return m.key(keyPress("enter"))
			}
			m.week.cursor = i
		}
	case m.tab == tabInbox:
		if i, ok := m.inbox.rowAt(line); ok {
			if i == m.inbox.cursor {
				return m.key(keyPress("enter"))
			}
			m.inbox.cursor = i
		}
	case m.tab == tabRules:
		if i, ok := m.rules.rowAt(line); ok {
			m.rules.cursor = i
		}
	case m.tab == tabPlan && !m.plan.adding:
		if i, ok := m.plan.rowAt(line); ok {
			m.plan.cursor = i
		}
	}
	return m, nil
}

// buttonRe matches a dialog button: "[y] accept", "[s] more specific".
var buttonRe = regexp.MustCompile(`\[(\S)\]( \S+)*`)

// buttonAt is the key of the button covering column x of a plain line.
func buttonAt(line string, x int) (string, bool) {
	for _, loc := range buttonRe.FindAllStringSubmatchIndex(line, -1) {
		from, to := ansi.StringWidth(line[:loc[0]]), ansi.StringWidth(line[:loc[1]])
		if x >= from && x < to {
			return line[loc[2]:loc[3]], true
		}
	}
	return "", false
}
