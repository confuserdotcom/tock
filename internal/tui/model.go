// Package tui is tock's terminal interface: today, week and inbox tabs over
// an app session. It never touches the database directly.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

const (
	tabToday = iota
	tabWeek
	tabInbox
	numTabs
)

// Refresh is how often the open tab reloads.
const Refresh = 5 * time.Second

// flashFor is how long a status message stays on the bottom line.
const flashFor = 5 * time.Second

// InboxDays is the inbox window, as in `tock inbox`.
const InboxDays = 7

// MinWidth and MinHeight are the smallest usable terminal.
const MinWidth, MinHeight = 60, 16

// Model is the root Bubble Tea model.
type Model struct {
	src  *source
	now  func() time.Time
	tick func() tea.Cmd // nil disables the refresh timer (tests)

	width, height int
	tab           int
	health        app.Health
	healthErr     error
	inbox         inboxTab
	today         todayTab
	week          weekTab

	picker *picker
	learn  *learnDlg
	help   bool
	score  bool // the today tab's score breakdown

	flash       string
	flashErr    bool
	flashUntil  time.Time
	loading     bool
	reloadAgain bool
}

// New returns the model for an open app. now is the clock used for "today"
// and status messages; it should be the app's clock.
func New(a *app.App, now func() time.Time) Model {
	return Model{
		src:  &source{a: a},
		now:  now,
		tick: func() tea.Cmd { return tea.Tick(Refresh, func(time.Time) tea.Msg { return tickMsg{} }) },
	}
}

type tickMsg struct{}

// loadedMsg carries one refresh.
type loadedMsg struct {
	health    app.Health
	healthErr error
	items     []classify.Item
	day       *report.Day
	week      *report.Week
	err       error
}

// doneMsg reports an action (tag, learn, ignore) for the status line.
type doneMsg struct {
	text string
	err  error
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.schedule())
}

func (m Model) schedule() tea.Cmd {
	if m.tick == nil {
		return nil
	}
	return m.tick()
}

// load reloads the rules, health, the inbox and the open tab's data.
func (m *Model) load() tea.Cmd {
	m.loading = true
	tab, dayDate, weekDate := m.tab, m.today.dateOr(m.now()), m.week.dateOr(m.now())
	src := m.src
	return func() tea.Msg {
		var msg loadedMsg
		src.do(func(a *app.App) {
			a.Reload() // a failure is kept in RulesErr and shown in the banner
			msg.health, msg.healthErr = a.Health()
			if msg.items, msg.err = a.Inbox(InboxDays); msg.err != nil {
				return
			}
			switch tab {
			case tabToday:
				d, err := a.Day(dayDate)
				msg.day, msg.err = &d, err
			case tabWeek:
				w, err := a.Week(weekDate)
				msg.week, msg.err = &w, err
			}
		})
		return msg
	}
}

// reload starts a load now, or right after the one in flight.
func (m *Model) reload() tea.Cmd {
	if m.loading {
		m.reloadAgain = true
		return nil
	}
	return m.load()
}

func (m *Model) setFlash(text string, isErr bool) {
	m.flash, m.flashErr, m.flashUntil = text, isErr, m.now().Add(flashFor)
}

// Update handles one message, then fits the scroll positions to the screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	mm := next.(Model)
	_, mm.today.timelineH = mm.today.layout(mm.bodyH())
	mm.today.clampScroll()
	mm.inbox.height = mm.bodyH() - inboxHeader
	mm.inbox.clampScroll()
	mm.week.clamp()
	return mm, cmd
}

// bodyH is the number of rows between the header (and banner) and the
// bottom line.
func (m Model) bodyH() int {
	h := m.height - 2
	if m.banner() != "" {
		h--
	}
	return max(h, 1)
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		if !m.flashUntil.IsZero() && !m.now().Before(m.flashUntil) {
			m.flash, m.flashUntil = "", time.Time{}
		}
		return m, tea.Batch(m.reload(), m.schedule())
	case loadedMsg:
		m.loading = false
		m.health, m.healthErr = msg.health, msg.healthErr
		if msg.err != nil {
			m.setFlash("load failed: "+msg.err.Error(), true)
		} else {
			m.inbox.set(msg.items)
			if msg.day != nil {
				m.today.set(*msg.day, m.now())
			}
			if msg.week != nil {
				m.week.set(*msg.week)
			}
		}
		if m.reloadAgain {
			m.reloadAgain = false
			return m, m.load()
		}
		return m, nil
	case doneMsg:
		if msg.err != nil {
			m.setFlash(msg.err.Error(), true)
		} else if msg.text != "" {
			m.setFlash(msg.text, false)
		}
		return m, m.reload()
	case proposedMsg:
		return m.openLearn(msg)
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	switch {
	case m.picker != nil:
		return m.pickerKey(msg)
	case m.learn != nil:
		return m.learnKey(k)
	case m.help:
		if k == "?" || k == "esc" || k == "q" {
			m.help = false
		}
		return m, nil
	case m.score:
		switch k {
		case "?":
			m.score, m.help = false, true
		case "esc", "q":
			m.score = false
		}
		return m, nil
	case m.tab == tabToday && m.today.filtering:
		m.today.filterKey(msg)
		return m, nil
	}
	switch k {
	case "q":
		return m, tea.Quit
	case "?":
		if m.tab == tabToday {
			m.score = true
		} else {
			m.help = true
		}
		return m, nil
	case "r":
		return m, m.reload()
	case "1", "2", "3":
		return m.switchTab(int(k[0] - '1'))
	case "tab":
		return m.switchTab((m.tab + 1) % numTabs)
	case "shift+tab":
		return m.switchTab((m.tab + numTabs - 1) % numTabs)
	}
	switch m.tab {
	case tabToday:
		return m.todayKey(k)
	case tabWeek:
		return m.weekKey(k)
	}
	return m.inboxKey(k)
}

func (m Model) switchTab(tab int) (tea.Model, tea.Cmd) {
	if tab == m.tab {
		return m, nil
	}
	m.tab = tab
	return m, m.reload()
}

// View renders the whole screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	if m.width == 0 {
		return ""
	}
	if m.width < MinWidth || m.height < MinHeight {
		return fmt.Sprintf("terminal too small (%d×%d needed)", MinWidth, MinHeight)
	}
	lines := []string{m.header()}
	if b := m.banner(); b != "" {
		lines = append(lines, sAccent.Render(fit(b, m.width)))
	}
	bodyH := m.bodyH()
	var body []string
	switch {
	case m.help:
		body = helpBody(m.width)
	case m.score:
		body = m.today.scoreView(m.width, bodyH, m.now().Location())
	case m.picker != nil:
		body = m.picker.view(m.width, bodyH)
	case m.learn != nil:
		body = m.learn.view(m.width)
	case m.tab == tabToday:
		body = m.today.view(m.width, bodyH, m.now())
	case m.tab == tabWeek:
		body = m.week.view(m.width, bodyH)
	default:
		body = m.inbox.view(m.width, bodyH)
	}
	if len(body) > bodyH {
		body = body[:bodyH]
	}
	for len(body) < bodyH {
		body = append(body, "")
	}
	lines = append(lines, body...)
	lines = append(lines, m.footer())
	return strings.Join(lines, "\n")
}

func (m Model) header() string {
	names := []string{"1 today", "2 week", "3 inbox"}
	if n := m.inbox.count(); n > 0 {
		names[tabInbox] = fmt.Sprintf("3 inbox %d", n)
	}
	parts := []string{sHead.Render("tock")}
	for i, n := range names {
		if i == m.tab {
			parts = append(parts, sHead.Underline(true).Render(n))
		} else {
			parts = append(parts, sDim.Render(n))
		}
	}
	left := strings.Join(parts, "   ")
	right := sDim.Render(m.now().Format("Mon 2 Jan 15:04")+" · ") + m.trackdText()
	return spread(left, right, m.width)
}

func (m Model) trackdText() string {
	if m.health.TrackdUp {
		return sDim.Render("trackd ok")
	}
	return sAccent.Render("trackd down")
}

// banner is the most important problem, or "".
func (m Model) banner() string {
	h := m.health
	switch {
	case m.healthErr != nil:
		return "cannot read trackd state: " + m.healthErr.Error()
	case !h.TrackdUp:
		return "trackd is not running — showing history. Restart: launchctl kickstart -k gui/$UID/com.tock.trackd"
	case h.RulesErr != nil:
		return "rule error, using the last good rules: " + h.RulesErr.Error()
	case h.ConfigErr != nil:
		return "config error, using the last good settings: " + h.ConfigErr.Error()
	case h.AXTrusted != nil && !*h.AXTrusted:
		return "no window titles: allow trackd in System Settings › Privacy & Security › Accessibility"
	case len(h.AutomationOff) > 0:
		return "no URLs from " + strings.Join(h.AutomationOff, ", ") + ": allow trackd in System Settings › Privacy & Security › Automation"
	}
	return ""
}

func (m Model) footer() string {
	if m.flash != "" {
		if m.flashErr {
			return sAccent.Render(fit(m.flash, m.width))
		}
		return sText.Render(fit(m.flash, m.width))
	}
	var hint string
	switch {
	case m.help:
		hint = "esc close"
	case m.score:
		hint = "? keys · esc close"
	case m.picker != nil:
		hint = "type to filter · ↑/↓ move · enter pick · esc cancel"
	case m.learn != nil:
		hint = m.learn.hint()
	case m.tab == tabToday:
		hint = m.today.hint()
	case m.tab == tabWeek:
		hint = "j/k day · h/l week · . this week · enter open day · ? help"
	default:
		hint = "enter learn rule · o tag once · x ignore · u undo · ? help"
	}
	return sDim.Render(fit(hint, m.width))
}
