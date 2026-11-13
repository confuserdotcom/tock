// Package tui is tock's terminal interface: today, week, inbox, focus, rules and plan
// tabs over an app session. It never touches the database directly; the plan tab edits
// the [plan] todo.txt file.
package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/plan"
	"tock/internal/report"
	"tock/internal/suggest"
)

const (
	tabToday = iota
	tabWeek
	tabInbox
	tabFocus
	tabRules
	tabPlan
	tabLabels
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
	noFocus       bool // the Focus group is off: no focus tab
	health        app.Health
	healthErr     error
	inbox         inboxTab
	today         todayTab
	week          weekTab
	focus         focusTab
	rules         rulesTab
	plan          planTab
	labels        labelsTab

	picker  *picker
	learn   *learnDlg
	start   *startDlg
	edit    *rangeDlg
	ask     *askDlg
	choose  *chooseDlg
	help    bool
	score   bool // the today tab's score breakdown
	palette *palette

	flash       string
	flashErr    bool
	flashUntil  time.Time
	loading     bool
	reloadAgain bool

	zones *zoneMap // the last frame's clickable zones; shared by copies
	menu  *ctxMenu // the right-click menu, nil = closed

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
		zones:     &zoneMap{},
	}
}

// WithTab opens on the named tab ("plan"); other names keep today.
func (m Model) WithTab(name string) Model {
	if name == "plan" {
		m.tab, m.plan.force = tabPlan, true
	}
	return m
}

type tickMsg struct{}

// loadedMsg carries one refresh.
type loadedMsg struct {
	noFocus   bool // the Focus group is off
	health    app.Health
	healthErr error
	items     []classify.Item
	small     []classify.Item // under [inbox] min_minutes
	day       *report.Day
	billed    map[int64]string // the day's views on an issued invoice, to its number
	week      *report.Week
	month     *report.Month
	focus     *report.Focus
	rules     *rulesSnap
	labels    []app.TreeCategory
	planCfg   config.Plan
	planLines []string
	planErr   error
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
			msg.planCfg = a.Config().Plan
			msg.noFocus = !a.Config().Focus.Enabled
			msg.health, msg.healthErr = a.Health()
			f, err := a.Focus(now)
			if err != nil {
				msg.err = err
				return
			}
			msg.focus = &f
			if msg.err = a.AutoAcceptEvery(InboxDays); msg.err != nil {
				return
			}
			if msg.items, msg.err = a.Inbox(InboxDays); msg.err != nil {
				return
			}
			msg.items, msg.small = classify.Floor(msg.items, a.Config().Inbox.MinMS)
			if msg.sugg, msg.err = a.Suggestions(); msg.err != nil {
				return
			}
			switch tab {
			case tabToday:
				d, err := a.Day(dayDate)
				msg.day, msg.err = &d, err
				if err == nil {
					msg.billed, msg.err = billedViews(a, d)
				}
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
			case tabLabels:
				msg.labels, msg.err = a.LabelTree(LabelDays)
			}
		})
		if tab == tabPlan && msg.planCfg.File != "" {
			msg.planLines, msg.planErr = plan.Read(msg.planCfg.File)
		}
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
	mm.today.timelineH = mm.today.layout(mm.width, mm.bodyH())
	mm.today.clampScroll()
	mm.inbox.height = mm.bodyH() - inboxHeader
	mm.inbox.clampScroll()
	mm.week.clamp()
	mm.rules.height = mm.bodyH()
	mm.rules.clampScroll()
	mm.plan.height = mm.bodyH()
	mm.plan.clampScroll()
	mm.labels.height = mm.bodyH()
	mm.labels.clampScroll()
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
		m.noFocus, m.today.noFocus = msg.noFocus, msg.noFocus
		if m.noFocus && m.tab == tabFocus {
			m.tab = tabToday
		}
		m.health, m.healthErr = msg.health, msg.healthErr
		if msg.err != nil {
			m.setFlash("load failed: "+msg.err.Error(), true)
		} else {
			m.inbox.set(msg.items, msg.small)
			m.inbox.sugg = msg.sugg
			m.inbox.recorded = msg.health.Recorded
			m.today.sugg, m.today.trackdUp = msg.sugg, msg.health.TrackdUp
			if msg.day != nil {
				m.today.billed = msg.billed
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
			if m.tab == tabLabels && msg.labels != nil {
				m.labels.set(msg.labels)
			}
			if m.tab == tabPlan {
				m.plan.set(msg.planCfg, msg.planLines, msg.planErr, m.now())
			}
		}
		clock := m.maybeClock()
		run := m.maybeSuggest()
		var detail tea.Cmd
		if m.tab == tabFocus {
			detail = m.loadDetail()
		}
		var planRun tea.Cmd
		if m.tab == tabPlan && m.plan.due(m.now()) {
			planRun = m.runCommand()
		}
		if m.reloadAgain {
			m.reloadAgain = false
			return m, tea.Batch(m.load(), clock, run, planRun, detail)
		}
		return m, tea.Batch(clock, run, planRun, detail)
	case detailMsg:
		return m.gotDetail(msg)
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
	case planCmdMsg:
		return m.planCommandDone(msg)
	case planEditedMsg:
		return m.planEdited(msg)
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.MouseClickMsg:
		return m.click(msg.Mouse())
	case tea.MouseMotionMsg:
		return m.motion(msg.Mouse())
	case tea.MouseWheelMsg:
		return m.wheel(msg.Mouse())
	}
	return m, nil
}

// View renders the whole screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// render draws the screen and records its zones in m.zones.
func (m Model) render() string {
	if m.zones == nil {
		return m.draw()
	}
	hover, tipBuf, zoneIDs = m.zones.hover, map[string]string{}, zoneIDs[:0]
	lines, hits := scanZones(strings.Split(m.draw(), "\n"))
	m.zones.hits, m.zones.tips = hits, tipBuf
	hover, tipBuf = "", nil
	return strings.Join(lines, "\n")
}

func (m Model) draw() string {
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
	if m.menu != nil {
		lines = paintAt(lines, m.menu.lines(), m.menu.x, m.menu.y, m.width)
	}
	return strings.Join(lines, "\n")
}

// body is the screen between the header (and banner) and the footer,
// exactly h lines.
func (m Model) body(bodyH int) []string {
	var top []string
	switch {
	case m.palette != nil:
		top = center2(m.palette.view(&m, m.width), m.width)
	case m.help:
		top = center2(m.helpBody(m.width), m.width)
	case m.score:
		return fitBody(m.today.scoreView(m.width, bodyH, m.now().Location()), bodyH)
	case m.picker != nil:
		top = m.picker.view(m.width, bodyH)
	case m.learn != nil:
		top = m.learn.view(m.width)
	case m.start != nil:
		top = m.start.view(m.width)
	case m.edit != nil:
		top = m.edit.view(m.width)
	case m.ask != nil:
		top = m.ask.view(m.width)
	case m.choose != nil:
		top = m.choose.view(m.width)
	case m.tab == tabPlan && m.plan.adding:
		top = m.plan.addView(m.width)
	}
	base := fitBody(m.tabBody(bodyH), bodyH)
	if top == nil {
		return base
	}
	return overlay(base, fitBody(top, bodyH), m.width)
}

// fitBody cuts or pads body to exactly h lines.
func fitBody(body []string, h int) []string {
	if len(body) > h {
		body = body[:h]
	}
	for len(body) < h {
		body = append(body, "")
	}
	return body
}

// tabBody is the open tab's view.
func (m Model) tabBody(bodyH int) []string {
	var body []string
	switch {
	case m.tab == tabToday:
		body = m.today.view(m.width, bodyH, m.now())
	case m.tab == tabWeek && m.week.month:
		body = m.week.monthView(m.width)
	case m.tab == tabWeek:
		body = m.week.view(m.width, bodyH)
	case m.tab == tabFocus:
		body = m.focus.view(m.width, bodyH, m.now())
	case m.tab == tabRules:
		body = m.rules.view(m.width, bodyH)
	case m.tab == tabPlan:
		body = m.plan.view(m.width, bodyH)
	case m.tab == tabLabels:
		body = m.labels.view(m.width, bodyH)
	default:
		body = m.inbox.view(m.width, bodyH)
	}
	return m.rowZones(body)
}

// rowZones makes each list row of the week, inbox, rules and plan tabs a
// row zone, found by the tab's own line maths. Today and labels mark their
// rows as they draw them.
func (m Model) rowZones(body []string) []string {
	var at func(line int) (int, bool)
	switch {
	case m.tab == tabWeek && !m.week.month:
		at = m.week.rowAt
	case m.tab == tabInbox:
		at = m.inbox.rowAt
	case m.tab == tabRules:
		at = m.rules.rowAt
	case m.tab == tabPlan && !m.plan.adding:
		at = m.plan.rowAt
	default:
		return body
	}
	for l := range body {
		if i, ok := at(l); ok {
			body[l] = z("row:"+fmt.Sprint(i), fit(body[l], m.width))
		}
	}
	return body
}

// TabName is the open tab's name, as in the header.
func (m Model) TabName() string { return tabTitles[m.tab] }

// tabTitles are the tabs' names, in tab order.
var tabTitles = [numTabs]string{"today", "week", "inbox", "focus", "rules", "plan", "labels"}
