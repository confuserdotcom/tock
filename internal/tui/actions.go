package tui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"tock/internal/report"
	"tock/internal/span"
)

// action is one thing the user can do. The table drives the footer, the
// help overlay and the command palette, so all three agree.
type action struct {
	key   string // as KeyPressMsg.String(); a "/"-joined pair is shown only
	name  string
	group string
	prio  int // footer order, lower first; 0 = not in the footer
	when  func(m *Model) bool
	// run replaces sending key (palette-only actions); nil sends key.
	run func(m Model) (tea.Model, tea.Cmd)
}

// nav reports a key pair like "j/k": listed in help, never run.
func (a action) nav() bool { return strings.Contains(a.key, "/") && a.key != "/" }

const (
	grToday = "Today"
	grWeek  = "Week"
	grMonth = "Month"
	grInbox = "Inbox"
	grFocus = "Focus"
	grRules = "Rules"
	grGo    = "Go"
	grApp   = "App"
)

// tabGroup is the action group of each tab.
var tabGroup = [numTabs]string{grToday, grWeek, grInbox, grFocus, grRules}

func onTab(tab int) func(m *Model) bool {
	return func(m *Model) bool { return m.tab == tab && !(tab == tabWeek && m.week.month) }
}

func always(*Model) bool { return true }

// todayRows reports the today tab showing a day with activity.
func todayRows(m *Model) bool { return m.tab == tabToday && len(m.today.day.Timeline) > 0 }

// todayRow is the selected today row when the today tab is open.
func todayRow(m *Model) *report.Row {
	if m.tab != tabToday {
		return nil
	}
	return m.today.selected()
}

func rowIs(f func(m *Model, r *report.Row) bool) func(m *Model) bool {
	return func(m *Model) bool {
		r := todayRow(m)
		return r != nil && f(m, r)
	}
}

func goTab(tab int) func(m Model) (tea.Model, tea.Cmd) {
	return func(m Model) (tea.Model, tea.Cmd) { return m.switchTab(tab) }
}

func startN(n int) func(m Model) (tea.Model, tea.Cmd) {
	return func(m Model) (tea.Model, tea.Cmd) {
		if m.focus.data.Open != nil {
			m.setFlash("a session is already running · x stops it", true)
			return m, nil
		}
		m.tab = tabFocus
		return m, m.startSession(n)
	}
}

func inAway(m *Model, r *report.Row) bool { return m.today.awayOf(r) != nil }

// actionTable is every action, in help order within each group.
func actionTable() []action {
	inbox := func(m *Model) bool { return m.tab == tabInbox && m.inbox.selected() != nil }
	month := func(m *Model) bool { return m.tab == tabWeek && m.week.month }
	return []action{
		{key: "enter", name: "label it", group: grToday, prio: 1,
			when: rowIs(func(m *Model, r *report.Row) bool { return m.today.enterAction(r, m.inbox.sugg) != "" })},
		{key: "a", name: "label time away", group: grToday, prio: 2,
			when: func(m *Model) bool { return m.tab == tabToday && len(m.today.day.Away) > 0 }},
		{key: "A", name: "leave time away idle", group: grToday, prio: 45,
			when: func(m *Model) bool { return m.tab == tabToday && len(m.today.day.Away) > 0 }},
		{key: "t", name: "tag", group: grToday, prio: 10,
			when: rowIs(func(m *Model, r *report.Row) bool { return !r.Idle })},
		{key: "t", name: "label away", group: grToday, prio: 10,
			when: rowIs(func(m *Model, r *report.Row) bool { return r.Idle && inAway(m, r) })},
		{key: "p", name: "set project", group: grToday, prio: 20,
			when: rowIs(func(m *Model, r *report.Row) bool { return !r.Idle })},
		{key: "T", name: "tag and learn a rule", group: grToday, prio: 30,
			when: rowIs(func(m *Model, r *report.Row) bool {
				return !r.Idle && r.App != span.AwayApp && r.App != span.MeetingApp
			})},
		{key: "j/k", name: "move", group: grToday},
		{key: "h", name: "previous day", group: grToday, prio: 50, when: onTab(tabToday)},
		{key: "l", name: "next day", group: grToday, prio: 51,
			when: func(m *Model) bool { return m.tab == tabToday && !m.today.isToday(m.now()) }},
		{key: ".", name: "back to today", group: grToday, prio: 52,
			when: func(m *Model) bool { return m.tab == tabToday && !m.today.isToday(m.now()) }},
		{key: "/", name: "filter rows", group: grToday, prio: 60, when: todayRows},
		{key: "i", name: "why this score", group: grToday, prio: 40, when: todayRows},
		{key: "g/G", name: "first / last row", group: grToday},

		{key: "j/k", name: "move between days", group: grWeek},
		{key: "enter", name: "open the day", group: grWeek, prio: 1, when: onTab(tabWeek)},
		{key: "h", name: "previous week", group: grWeek, prio: 10, when: onTab(tabWeek)},
		{key: "l", name: "next week", group: grWeek, prio: 11, when: onTab(tabWeek)},
		{key: ".", name: "this week", group: grWeek, prio: 12, when: onTab(tabWeek)},
		{key: "m", name: "month trends", group: grWeek, prio: 5, when: onTab(tabWeek)},

		{key: "h", name: "previous month", group: grMonth, prio: 10, when: month},
		{key: "l", name: "next month", group: grMonth, prio: 11, when: month},
		{key: ".", name: "this month", group: grMonth, prio: 12, when: month},
		{key: "m", name: "back to the week", group: grMonth, prio: 5, when: month},

		{key: "enter", name: "label, learn a rule", group: grInbox, prio: 1, when: inbox},
		{key: "T", name: "pick a label", group: grInbox, when: inbox},
		{key: "a", name: "accept the suggestion", group: grInbox, prio: 2,
			when: func(m *Model) bool {
				it := m.inbox.selected()
				if m.tab != tabInbox || it == nil {
					return false
				}
				_, ok := m.inbox.sugg[it.Key]
				return ok
			}},
		{key: "o", name: "tag once, no rule", group: grInbox, prio: 10, when: inbox},
		{key: "x", name: "ignore", group: grInbox, prio: 20, when: inbox},
		{key: "u", name: "undo ignore", group: grInbox, prio: 30,
			when: func(m *Model) bool { return m.tab == tabInbox && len(m.inbox.ignored) > 0 }},
		{key: "j/k", name: "move", group: grInbox},

		{key: "s", name: "start a session", group: grFocus, prio: 1,
			when: func(m *Model) bool { return m.tab == tabFocus && m.focus.data.Open == nil }},
		{key: "x", name: "stop the session", group: grFocus, prio: 1,
			when: func(m *Model) bool { return m.tab == tabFocus && m.focus.data.Open != nil }},
		{name: "start a 25 min session", group: grFocus, run: startN(25),
			when: func(m *Model) bool { return m.focus.data.Open == nil }},
		{name: "start a 50 min session", group: grFocus, run: startN(50),
			when: func(m *Model) bool { return m.focus.data.Open == nil }},
		{name: "start a 90 min session", group: grFocus, run: startN(90),
			when: func(m *Model) bool { return m.focus.data.Open == nil }},
		{name: "stop the session", group: grFocus,
			when: func(m *Model) bool { return m.tab != tabFocus && m.focus.data.Open != nil },
			run:  func(m Model) (tea.Model, tea.Cmd) { return m, m.stopSession() }},

		{key: "e", name: "edit rules.toml", group: grRules, prio: 1, when: onTab(tabRules)},
		{key: "J/K", name: "move rule down/up", group: grRules},
		{key: "j/k", name: "move", group: grRules},

		{key: "1", name: "today", group: grGo, when: always, run: goTab(tabToday)},
		{key: "2", name: "week", group: grGo, when: always, run: goTab(tabWeek)},
		{key: "3", name: "inbox", group: grGo, when: always, run: goTab(tabInbox)},
		{key: "4", name: "focus", group: grGo, when: always, run: goTab(tabFocus)},
		{key: "5", name: "rules", group: grGo, when: always, run: goTab(tabRules)},
		{key: "tab", name: "next tab", group: grGo},

		{key: "?", name: "help", group: grApp, prio: 98, when: always},
		{key: ":", name: "commands", group: grApp, prio: 99, when: always},
		{key: "r", name: "refresh", group: grApp, when: always},
		{key: "q", name: "quit", group: grApp, when: always},
	}
}

// available is the runnable actions of the current state, in table order.
func (m *Model) available() []action {
	var out []action
	for _, a := range actionTable() {
		if !a.nav() && a.when != nil && a.when(m) {
			out = append(out, a)
		}
	}
	return out
}

// footerActions is the footer's actions: available ones with a prio, by
// prio, fitted to w cells with the lowest priorities dropped first. help
// and commands always stay.
func (m *Model) footerActions(w int) []action {
	var acts []action
	for _, a := range m.available() {
		if a.prio > 0 {
			acts = append(acts, a)
		}
	}
	sort.SliceStable(acts, func(i, j int) bool { return acts[i].prio < acts[j].prio })
	for len(acts) > 2 && ansi.StringWidth(hintOf(acts)) > w {
		acts = append(acts[:len(acts)-3], acts[len(acts)-2:]...)
	}
	return acts
}

// hintOf is the plain footer text of acts: "t tag · p set project".
func hintOf(acts []action) string {
	parts := make([]string, len(acts))
	for i, a := range acts {
		parts[i] = a.key + " " + a.name
	}
	return strings.Join(parts, " · ")
}

// styledHint is hintOf with keycaps.
func styledHint(acts []action) string {
	parts := make([]string, len(acts))
	for i, a := range acts {
		parts[i] = keycap(a.key, a.name)
	}
	return strings.Join(parts, sFaint.Render(" · "))
}

// runAction runs a: its run func, else its key.
func (m Model) runAction(a action) (tea.Model, tea.Cmd) {
	if a.run != nil {
		return a.run(m)
	}
	return m.key(keyPress(a.key))
}
