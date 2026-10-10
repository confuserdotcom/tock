// Package tui is tock's terminal interface: today, week, inbox, focus and rules tabs over
// an app session. It never touches the database directly.
package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/suggest"
)

const (
	tabToday = iota
	tabWeek
	tabInbox
	tabFocus
	tabRules
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
	src       *source
	now       func() time.Time
	tick      func() tea.Cmd // nil disables the refresh timer (tests)
	clockTick func() tea.Cmd // nil disables the countdown redraw (tests)
	clockOn   bool
	// exec runs an external program (the editor) with the terminal; tests
	// substitute it.
	exec func(*exec.Cmd, tea.ExecCallback) tea.Cmd

	width, height int
	tab           int
	health        app.Health
	healthErr     error
	inbox         inboxTab
	today         todayTab
	week          weekTab
	focus         focusTab
	rules         rulesTab

	picker *picker
	learn  *learnDlg
	start  *startDlg
	help   bool
	score  bool // the today tab's score breakdown

	flash       string
	flashErr    bool
	flashUntil  time.Time
	loading     bool
	reloadAgain bool

	suggester  suggest.Client // nil: no key, no suggestions
	suggesting bool           // a background run is in flight
	suggestAt  time.Time      // earliest start of the next run
}

// New returns the model for an open app. now is the clock used for "today"
// and status messages; it should be the app's clock.
func New(a *app.App, now func() time.Time) Model {
	return Model{
		src:       &source{a: a},
		now:       now,
		tick:      func() tea.Cmd { return tea.Tick(Refresh, func(time.Time) tea.Msg { return tickMsg{} }) },
		clockTick: func() tea.Cmd { return tea.Tick(clockEvery, func(time.Time) tea.Msg { return clockMsg{} }) },
		exec:      tea.ExecProcess,
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
	month     *report.Month
	focus     *report.Focus
	rules     *rulesSnap
	sugg      map[string]classify.Want
	err       error
}

// doneMsg reports an action (tag, learn, ignore) for the status line.
type doneMsg struct {
	text string
	err  error
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.schedule(), tea.RequestBackgroundColor)
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
	now := m.now()
	tab, dayDate, weekDate := m.tab, m.today.dateOr(now), m.week.dateOr(now)
	month, monthDate := m.week.month, m.week.monthOr(now)
	src := m.src
	return func() tea.Msg {
		var msg loadedMsg
		src.do(func(a *app.App) {
			a.Reload() // a failure is kept in RulesErr and shown in the banner
			msg.health, msg.healthErr = a.Health()
			f, err := a.Focus(now)
			if err != nil {
				msg.err = err
				return
			}
			msg.focus = &f
			if msg.items, msg.err = a.Inbox(InboxDays); msg.err != nil {
				return
			}
			if msg.sugg, msg.err = a.Suggestions(); msg.err != nil {
				return
			}
			switch tab {
			case tabToday:
				d, err := a.Day(dayDate)
				msg.day, msg.err = &d, err
			case tabWeek:
				if month {
					mo, err := a.Month(monthDate)
					msg.month, msg.err = &mo, err
					break
				}
				w, err := a.Week(weekDate)
				msg.week, msg.err = &w, err
			case tabRules:
				msg.rules = snapRules(a)
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
	mm.rules.height = mm.bodyH()
	mm.rules.clampScroll()
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
	case tea.BackgroundColorMsg:
		setTheme(msg.IsDark())
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
			m.inbox.sugg = msg.sugg
			if msg.day != nil {
				m.today.set(*msg.day, m.now())
			}
			if msg.week != nil {
				m.week.set(*msg.week)
			}
			if msg.month != nil {
				m.week.setMonth(*msg.month)
			}
			if msg.focus != nil {
				m.focus.set(*msg.focus)
			}
			if msg.rules != nil {
				m.rules.set(*msg.rules)
			}
		}
		clock := m.maybeClock()
		run := m.maybeSuggest()
		if m.reloadAgain {
			m.reloadAgain = false
			return m, tea.Batch(m.load(), clock, run)
		}
		return m, tea.Batch(clock, run)
	case clockMsg:
		m.clockOn = false
		return m, m.maybeClock()
	case suggestedMsg:
		return m.suggested(msg)
	case doneMsg:
		if msg.err != nil {
			m.setFlash(msg.err.Error(), true)
		} else if msg.text != "" {
			m.setFlash(msg.text, false)
		}
		return m, m.reload()
	case movedMsg:
		if msg.done.err == nil {
			m.rules.cursor = msg.row
			m.rules.clampScroll()
		}
		return m.update(msg.done)
	case proposedMsg:
		return m.openLearn(msg)
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.MouseClickMsg:
		return m.click(msg.Mouse())
	case tea.MouseWheelMsg:
		return m.wheel(msg.Mouse())
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
	case m.start != nil:
		return m.startKey(msg)
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
	case "1", "2", "3", "4", "5":
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
	case tabFocus:
		return m.focusKey(k)
	case tabRules:
		return m.rulesKey(k)
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
	v.MouseMode = tea.MouseModeCellMotion
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
	lines = append(lines, m.body(m.bodyH())...)
	lines = append(lines, m.footer())
	return strings.Join(lines, "\n")
}

// body is the screen between the header (and banner) and the footer,
// exactly h lines.
func (m Model) body(bodyH int) []string {
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
	case m.start != nil:
		body = m.start.view(m.width)
	case m.tab == tabToday:
		body = m.today.view(m.width, bodyH, m.now())
	case m.tab == tabWeek && m.week.month:
		body = m.week.monthView(m.width)
	case m.tab == tabWeek:
		body = m.week.view(m.width, bodyH)
	case m.tab == tabFocus:
		body = m.focus.view(m.width, m.now())
	case m.tab == tabRules:
		body = m.rules.view(m.width, bodyH)
	default:
		body = m.inbox.view(m.width, bodyH)
	}
	if len(body) > bodyH {
		body = body[:bodyH]
	}
	for len(body) < bodyH {
		body = append(body, "")
	}
	return body
}

// headerGap separates the header's names.
const headerGap = "   "

// tabNames are the header's tab names, in tab order. The inbox carries its
// count as a superscript badge.
func (m Model) tabNames() []string {
	names := []string{"today", "week", "inbox", "focus", "rules"}
	if n := m.inbox.count(); n > 0 {
		names[tabInbox] = "inbox " + superscript(n)
	}
	return names
}

// superscript writes n in superscript digits: 12 → ¹².
func superscript(n int) string {
	const sup = "⁰¹²³⁴⁵⁶⁷⁸⁹"
	digits := []rune(sup)
	var b strings.Builder
	for _, c := range fmt.Sprint(n) {
		b.WriteRune(digits[c-'0'])
	}
	return b.String()
}

func (m Model) header() string {
	names := m.tabNames()
	parts := []string{sBrand.Bold(true).Render("tock")}
	for i, n := range names {
		switch {
		case i == m.tab:
			parts = append(parts, sHead.Underline(true).Render(n))
		case i == tabInbox && m.inbox.count() > 0:
			name, badge, _ := strings.Cut(n, " ")
			parts = append(parts, sDim.Render(name+" ")+sUnl.Render(badge))
		default:
			parts = append(parts, sDim.Render(n))
		}
	}
	left := strings.Join(parts, headerGap)
	right := sDim.Render(m.now().Format("Mon 2 Jan  15:04")+"  ") + m.trackdText()
	if s := m.focus.data.Open; s != nil {
		right = pill("◷ "+report.Countdown(s.PlannedMS-m.now().UnixMilli()), sBrand) + "  " + right
	}
	return spread(left, right, m.width)
}

func (m Model) trackdText() string {
	if m.health.TrackdUp {
		return sGood.Render("●") + sDim.Render(" trackd")
	}
	return sAccent.Render("● trackd down")
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
	return sDim.Render(fit(m.hint(), m.width))
}

// hint is the footer's key list for the current view.
func (m Model) hint() string {
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
	case m.start != nil:
		hint = "↑/↓ move · digits custom length · enter start · esc cancel"
	case m.tab == tabToday:
		hint = m.today.hint()
	case m.tab == tabWeek && m.week.month:
		hint = "h/l month · . this month · m week · ? help"
	case m.tab == tabWeek:
		hint = "j/k day · h/l week · . this week · enter open day · m month · ? help"
	case m.tab == tabFocus:
		hint = "s start · x stop · ? help"
	case m.tab == tabRules:
		hint = "j/k move · J/K reorder · e edit in $EDITOR · ? help"
	default:
		hint = "enter learn rule · a accept suggestion · o tag once · x ignore · u undo · ? help"
	}
	return hint
}
