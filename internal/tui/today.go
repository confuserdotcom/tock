package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/report"
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

func (t *todayTab) hint() string {
	if t.filtering {
		return "filter: " + t.filter + "▏ · enter keep · esc clear"
	}
	return "j/k move · h/l day · t tag · p project · T learn · / filter · ? score"
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
	case "t", "p", "T":
		r := t.selected()
		switch {
		case r == nil:
		case r.Idle:
			m.setFlash("idle time has no label", true)
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

// layout splits a body of h rows: how many category rows fit and how tall
// the timeline is. view and the key handlers must agree on it.
func (t *todayTab) layout(h int) (catRows, timelineH int) {
	const minTimeline = 4
	top := 3 + len(t.notes()) + 1 + len(t.day.Hours.MS) + 1 // title, totals, notes, blank, axis, kinds, blank
	catRows = max(len(t.day.Categories), 1)
	catRows = min(catRows, max(h-top-1-minTimeline, 1))
	return catRows, max(h-top-catRows-1, 1)
}

// rowAt is the timeline row shown on body line line of a body h lines
// tall, if any.
func (t *todayTab) rowAt(line, h int) (int, bool) {
	if !t.loaded {
		return 0, false
	}
	catRows, timelineH := t.layout(h)
	start := 3 + len(t.notes()) + 1 + len(t.day.Hours.MS) + 1 + catRows + 1
	i := t.top + line - start
	if line < start || line >= start+timelineH || i >= len(t.rows()) {
		return 0, false
	}
	return i, true
}

// view renders the tab body in w×h cells.
func (t todayTab) view(w, h int, now time.Time) []string {
	if !t.loaded {
		return []string{"", " loading…"}
	}
	d := t.day
	title := "Today"
	if !t.isToday(now) {
		title = d.Date.Format("Mon 2 Jan")
	}
	var out []string
	out = append(out, " "+sHead.Render(title)+"  "+scoreText(d))
	out = append(out, " "+statsLine(d))
	for _, n := range t.notes() {
		out = append(out, fit(n, w))
	}
	out = append(out, "")
	out = append(out, heatmap(d, w)...)
	out = append(out, "")

	catRows, timelineH := t.layout(h)
	var maxMS int64
	for _, c := range d.Categories {
		maxMS = max(maxMS, c.MS)
	}
	for _, c := range d.Categories[:min(len(d.Categories), catRows)] {
		extra := projects(c.Projects)
		if c.Name == report.Unclassified && len(c.Items) > 0 {
			extra = plural(len(c.Items), "key") + " → 3 inbox"
		}
		bar := kindStyle(c.Kind).Render(report.Pad(report.Bar(c.MS, maxMS, 12), 12))
		line := " " + fit(c.Name, 15) + fitRight(report.Dur(c.MS), 5) + "  " + bar + "  " + sDim.Render(extra)
		out = append(out, line)
	}
	if len(d.Categories) == 0 {
		out = append(out, sDim.Render(" no activity"))
	}
	out = append(out, "")

	rs := t.rows()
	end := min(t.top+timelineH, len(rs))
	for i := t.top; i < end; i++ {
		out = append(out, timelineRow(rs[i], w, i == t.cursor, now.Location()))
	}
	if len(rs) == 0 && t.filter != "" {
		out = append(out, sDim.Render(fmt.Sprintf(" no rows match %q", t.filter)))
	}
	return out
}

// notes are the lines between the stats line and the heatmap: the goals.
func (t *todayTab) notes() []string {
	var out []string
	if len(t.day.Goals) > 0 {
		out = append(out, goalsLine(t.day.Goals))
	}
	return out
}

// goalsLine is report.GoalsLine with each goal styled by its state.
func goalsLine(rows []report.GoalRow) string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		st := sText
		switch r.State {
		case report.GoalMet, report.GoalOK:
			st = sDim
		case report.GoalOver:
			st = sAccent
		}
		parts[i] = st.Render(report.GoalText(r))
	}
	return " " + sHead.Render("goals") + "  " + strings.Join(parts, sDim.Render(" · "))
}

// scoreText is report.ScoreLine with the score in the accent colour.
func scoreText(d report.Day) string {
	first, rest, _ := strings.Cut(report.ScoreLine(d), " · ")
	return sAccent.Render(first) + sDim.Render(" · ") + sText.Render(rest)
}

func statsLine(d report.Day) string {
	parts := []string{"tracked " + report.Dur(d.TrackedMS)}
	for _, k := range report.Kinds {
		if ms := d.ByKind[k]; ms > 0 {
			parts = append(parts, kindStyle(k).Render(kindName(k)+" "+report.Dur(ms)))
		}
	}
	parts = append(parts, sFaint.Render("idle "+report.Dur(d.IdleMS)))
	return strings.Join(parts, sDim.Render(" · "))
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

// heatmap draws hour labels and one row per kind with time, two cells per
// hour.
func heatmap(d report.Day, w int) []string {
	const label = 11
	hrs := d.Hours
	n := hrs.Last - hrs.First + 1
	cell := 2
	if label+n*cell > w-1 {
		cell = 1
	}
	step := 1
	for step*cell < 3 {
		step++
	}
	axis := []byte(strings.Repeat(" ", n*cell+2))
	for h := hrs.First; h <= hrs.Last; h += step {
		pos := (h - hrs.First) * cell
		copy(axis[pos:], fmt.Sprintf("%02d", h))
	}
	out := []string{" " + strings.Repeat(" ", label) + sFaint.Render(strings.TrimRight(string(axis), " "))}
	for _, k := range report.Kinds {
		hours, ok := hrs.MS[k]
		if !ok {
			continue
		}
		var b strings.Builder
		for h := hrs.First; h <= hrs.Last; h++ {
			b.WriteString(strings.Repeat(shade(hours[h]), cell))
		}
		out = append(out, " "+fit(kindName(k), label)+kindStyle(k).Render(b.String()))
	}
	return out
}

func timelineRow(r report.Row, w int, sel bool, loc *time.Location) string {
	start := time.UnixMilli(r.Start).In(loc).Format("15:04")
	end := time.UnixMilli(r.End).In(loc).Format("15:04")
	mark := " "
	if sel {
		mark = "▶"
	}
	label := fit(rowLabel(r), 18)
	switch {
	case r.Idle:
		label = sFaint.Render(label)
	case r.Label.Category == "" && r.Label.Project == "":
		label = sAccent.Render(label)
	default:
		label = kindStyle(r.Label.Kind).Render(label)
	}
	fixed := 1 + 11 + 2 + 5 + 2 + 18 + 2 + 10 + 2
	text := fit(r.Text, max(w-fixed, 0))
	line := mark + start + "–" + end + "  " + fitRight(report.Dur(r.End-r.Start), 5) + "  " + label + "  " +
		fit(r.App, 10) + "  " + sDim.Render(text)
	if sel {
		return sSel.Render(line)
	}
	return line
}
