package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/span"
)

// todayTab shows one day: stats, heatmap, category bars and the timeline.
type todayTab struct {
	date      time.Time // midnight of the shown day; zero = follow today
	day       report.Day
	loaded    bool
	cursor    int // index into rows()
	top       int // first visible timeline row
	follow    bool
	filter    string
	filtering bool
	timelineH int // timeline rows that fit; set by Model.Update
	// sugg are the inbox suggestions by key and trackdUp trackd's state,
	// both copied from the model on each load.
	sugg     map[string]classify.Want
	trackdUp bool
}

func (t *todayTab) dateOr(now time.Time) time.Time {
	if t.date.IsZero() {
		return now
	}
	return t.date
}

func (t *todayTab) isToday(now time.Time) bool {
	return t.date.IsZero() || sameDay(t.date, now)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.In(a.Location()).Date()
	return ay == by && am == bm && ad == bd
}

// rows is the timeline after the filter.
func (t *todayTab) rows() []report.Row {
	if t.filter == "" {
		return t.day.Timeline
	}
	q := strings.ToLower(t.filter)
	var out []report.Row
	for _, r := range t.day.Timeline {
		hay := strings.ToLower(rowLabel(r) + " " + r.App + " " + r.Text)
		if strings.Contains(hay, q) {
			out = append(out, r)
		}
	}
	return out
}

// set installs a new day, keeping the cursor on the same row.
func (t *todayTab) set(d report.Day, now time.Time) {
	var keepID, keepStart int64 = -1, 0
	if rs := t.rows(); t.loaded && t.cursor < len(rs) && !t.follow && sameDay(t.day.Date, d.Date) {
		keepID, keepStart = rs[t.cursor].IDs[0], rs[t.cursor].Start
	}
	first := !t.loaded || !sameDay(t.day.Date, d.Date)
	t.day, t.loaded = d, true
	rs := t.rows()
	switch {
	case first || t.follow || keepID < 0:
		t.cursor = max(len(rs)-1, 0)
		t.follow = true
	default:
		t.cursor = nearestRow(rs, keepID, keepStart)
	}
	t.clampScroll()
}

// nearestRow finds the row holding id, else the last row starting at or
// before start.
func nearestRow(rs []report.Row, id, start int64) int {
	best := 0
	for i, r := range rs {
		for _, x := range r.IDs {
			if x == id {
				return i
			}
		}
		if r.Start <= start {
			best = i
		}
	}
	return best
}

func (t *todayTab) move(delta int) {
	n := len(t.rows())
	if n == 0 {
		return
	}
	t.cursor = min(max(t.cursor+delta, 0), n-1)
	t.follow = t.cursor == n-1
	t.clampScroll()
}

func (t *todayTab) clampScroll() {
	h := max(t.timelineH, 1)
	if t.cursor < t.top {
		t.top = t.cursor
	}
	if t.cursor >= t.top+h {
		t.top = t.cursor - h + 1
	}
	t.top = max(min(t.top, len(t.rows())-h), 0)
}

// selected is the row under the cursor, or nil.
func (t *todayTab) selected() *report.Row {
	rs := t.rows()
	if t.cursor < 0 || t.cursor >= len(rs) {
		return nil
	}
	return &rs[t.cursor]
}

func (t *todayTab) filterKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		t.filter, t.filtering = "", false
	case "enter":
		t.filtering = false
	case "backspace":
		if r := []rune(t.filter); len(r) > 0 {
			t.filter = string(r[:len(r)-1])
		}
	default:
		if msg.Text != "" {
			t.filter += msg.Text
		}
	}
	t.cursor = max(len(t.rows())-1, 0)
	t.follow = true
	t.top = 0
	t.clampScroll()
}

// hint is the footer while the filter is typed.
func (t *todayTab) hint() string {
	return "filter: " + t.filter + "▏ · enter keep · esc clear"
}

// awayOf is the pending absence that holds idle row r, or nil.
func (t *todayTab) awayOf(r *report.Row) *classify.Interval {
	if !r.Idle {
		return nil
	}
	for i, iv := range t.day.Away {
		if r.Start >= iv.From && r.End <= iv.To {
			return &t.day.Away[i]
		}
	}
	return nil
}

// enterAction is what enter does on row r: "accept" its suggestion, "tag"
// an unlabelled row, label the "away" time of an idle row, or "" nothing.
func (t *todayTab) enterAction(r *report.Row, sugg map[string]classify.Want) string {
	switch {
	case r.Idle:
		if t.awayOf(r) != nil {
			return "away"
		}
	case r.Label.Category == "" && r.App != span.AwayApp && r.App != span.MeetingApp:
		if _, ok := sugg[r.Key]; ok {
			return "accept"
		}
		return "tag"
	}
	return ""
}

// awayWhat names an absence: "12:25–14:50 (2h25)".
func awayWhat(iv classify.Interval, loc *time.Location) string {
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	return clock(iv.From) + "–" + clock(iv.To) + " (" + report.Dur(iv.To-iv.From) + ")"
}

func (m Model) todayKey(k string) (tea.Model, tea.Cmd) {
	t := &m.today
	half := max(t.timelineH/2, 1)
	switch k {
	case "j", "down":
		t.move(1)
	case "k", "up":
		t.move(-1)
	case "g", "home":
		t.move(-len(t.rows()))
	case "G", "end":
		t.move(len(t.rows()))
	case "ctrl+d":
		t.move(half)
	case "ctrl+u":
		t.move(-half)
	case "/":
		t.filtering = true
	case "h":
		t.date = m.today.dateOr(m.now()).AddDate(0, 0, -1)
		t.loaded = false
		return m, m.reload()
	case "l":
		if t.isToday(m.now()) {
			return m, nil
		}
		next := t.date.AddDate(0, 0, 1)
		if sameDay(next, m.now()) {
			next = time.Time{}
		}
		t.date, t.loaded = next, false
		return m, m.reload()
	case ".":
		if !t.date.IsZero() {
			t.date, t.loaded = time.Time{}, false
			return m, m.reload()
		}
	case "enter":
		r := t.selected()
		if r == nil {
			return m, nil
		}
		switch t.enterAction(r, m.inbox.sugg) {
		case "accept":
			return m, m.propose(r.Key, m.inbox.sugg[r.Key])
		case "tag":
			m.picker = newPicker(m.src, pickTag, r.IDs, "", spansWhat(len(r.IDs), r.End-r.Start))
		case "away":
			iv := *t.awayOf(r)
			m.picker = newAwayPicker(m.src, iv, awayWhat(iv, m.now().Location()))
		}
		return m, nil
	case "a", "A":
		n := len(t.day.Away)
		if n == 0 {
			m.setFlash("no time away to label", true)
			return m, nil
		}
		iv := t.day.Away[n-1]
		if k == "a" {
			m.picker = newAwayPicker(m.src, iv, awayWhat(iv, m.now().Location()))
			return m, nil
		}
		what := awayWhat(iv, m.now().Location())
		return m, m.act(func(a *app.App) (string, error) {
			return "left " + what + " idle", a.Answer(iv, classify.Want{})
		})
	case "t", "p", "T":
		r := t.selected()
		switch {
		case r == nil:
		case r.Idle && k == "t":
			if iv := t.awayOf(r); iv != nil {
				m.picker = newAwayPicker(m.src, *iv, awayWhat(*iv, m.now().Location()))
				return m, nil
			}
			m.setFlash("not an away period: too short, too long or already answered", true)
		case r.Idle:
			m.setFlash("idle time has no label", true)
		case r.App == span.AwayApp && k == "T":
			m.setFlash("away time has no rules: t to relabel it", true)
		case r.App == span.MeetingApp && k == "T":
			m.setFlash("meetings are learned by hand: a [[rule]] with bundle tock.meeting; t to relabel this one", true)
		case k == "t":
			m.picker = newPicker(m.src, pickTag, r.IDs, "", spansWhat(len(r.IDs), r.End-r.Start))
		case k == "p":
			m.picker = newPicker(m.src, pickProject, r.IDs, "", spansWhat(len(r.IDs), r.End-r.Start))
		default:
			m.picker = newPicker(m.src, pickLearn, nil, r.Key, "")
		}
	}
	return m, nil
}

func spansWhat(n int, ms int64) string {
	return fmt.Sprintf("%s (%s)", plural(n, "span"), report.Dur(ms))
}

// plural renders "1 key", "3 keys".
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func rowLabel(r report.Row) string {
	switch {
	case r.Idle:
		return "idle"
	case r.Label.Category == "" && r.Label.Project == "":
		return "—"
	}
	return report.LabelText(r.Label.Category, r.Label.Project)
}

func projects(p []report.NameMS) string {
	var parts []string
	for _, n := range p[:min(len(p), 3)] {
		parts = append(parts, n.Name+" "+report.Dur(n.MS))
	}
	return strings.Join(parts, " · ")
}

// shade is the heatmap glyph for ms of one hour.
func shade(ms int64) string {
	switch m := ms / 60_000; {
	case ms <= 0:
		return " "
	case m < 15:
		return "░"
	case m < 30:
		return "▒"
	case m < 45:
		return "▓"
	}
	return "█"
}
