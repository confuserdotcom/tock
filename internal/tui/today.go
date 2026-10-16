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
	full      bool // every row; off, labelled rows in a row with one label merge (c)
	timelineH int  // timeline rows that fit; set by Model.Update
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

// rows is the timeline after the filter, compact unless full.
func (t *todayTab) rows() []report.Row {
	rs := t.day.Timeline
	if !t.full {
		rs = compact(rs)
	}
	if t.filter == "" {
		return rs
	}
	q := strings.ToLower(t.filter)
	var out []report.Row
	for _, r := range rs {
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

// jumpTo selects the row that holds ms, else the last one starting before it.
func (t *todayTab) jumpTo(ms int64) {
	rs := t.rows()
	if len(rs) == 0 {
		return
	}
	best := 0
	for i, r := range rs {
		if r.Start <= ms {
			best = i
		}
		if r.Start <= ms && ms < r.End {
			break
		}
	}
	t.cursor, t.follow = best, best == len(rs)-1
	t.clampScroll()
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
	case remoteRow(r):
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

// remoteRow reports a row of another machine's activity (tock sync).
func remoteRow(r *report.Row) bool { return len(r.IDs) > 0 && app.IsRemoteView(r.IDs[0]) }

// awayWhat names an absence: "12:25–14:50 (2h25)".
// before is the label of what the pending absence iv follows, or zero.
func (t *todayTab) before(iv classify.Interval) classify.Want {
	for i, a := range t.day.Away {
		if a == iv && i < len(t.day.AwayBefore) {
			return t.day.AwayBefore[i]
		}
	}
	return classify.Want{}
}

// awayPicker labels the absence iv; what it follows leads the quick
// options.
func (m *Model) awayPicker(iv classify.Interval) *picker {
	p := newAwayPicker(m.src, iv, awayWhat(iv, m.now().Location()))
	if b := m.today.before(iv); b.Category != "" {
		p.quick = append(p.quick, option{name: report.LabelText(b.Category, b.Project), note: "◷ before", want: &b})
	}
	return p.withQuick(m, "")
}

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
	case "c":
		t.full = !t.full
		t.cursor = min(t.cursor, max(len(t.rows())-1, 0))
		t.clampScroll()
		return m, nil
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
			m.picker = newPicker(m.src, pickTag, r.IDs, "", spansWhat(len(r.IDs), r.End-r.Start)).withQuick(&m, r.Key)
		case "away":
			iv := *t.awayOf(r)
			m.picker = m.awayPicker(iv)
		}
		return m, nil
	case "d":
		if r := t.selected(); r != nil && !r.Idle {
			return m, m.deleteRow(r)
		}
	case "s":
		r := t.selected()
		if r == nil || r.Idle || r.End-r.Start < 2*60_000 {
			m.setFlash("pick a row of at least 2 min to split", true)
			return m, nil
		}
		mid := (r.Start + (r.End-r.Start)/2) / 60_000 * 60_000
		m.edit = newRangeDlg(classify.EditLabel, "Split: label part of it", mid, r.End, t.day.Date)
		return m, nil
	case "+":
		from, to := m.now().UnixMilli()-30*60_000, m.now().UnixMilli()
		if r := t.selected(); r != nil && r.Idle {
			from, to = r.Start, r.End
		} else if !t.isToday(m.now()) {
			from = t.day.Date.Add(9 * time.Hour).UnixMilli()
			to = from + 60*60_000
		}
		m.edit = newRangeDlg(classify.EditAdd, "Add time", from, to, t.day.Date)
		return m, nil
	case "a", "A", "y":
		n := len(t.day.Away)
		if n == 0 {
			m.setFlash("no time away to label", true)
			return m, nil
		}
		iv := t.day.Away[n-1]
		if k == "y" {
			b := t.before(iv)
			if b.Category == "" {
				m.setFlash("nothing labelled right before it: a to label it", true)
				return m, nil
			}
			what := awayWhat(iv, m.now().Location())
			return m, m.act(func(a *app.App) (string, error) {
				return "labelled away " + what + " → " + report.LabelText(b.Category, b.Project), a.Answer(iv, b)
			})
		}
		if k == "a" {
			m.picker = m.awayPicker(iv)
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
				m.picker = m.awayPicker(*iv)
				return m, nil
			}
			m.setFlash("not an away period: too short, too long or already answered", true)
		case r.Idle:
			m.setFlash("idle time has no label", true)
		case remoteRow(r):
			m.setFlash("recorded on another machine: label it there", true)
		case r.App == span.AwayApp && k == "T":
			m.setFlash("away time has no rules: t to relabel it", true)
		case r.App == span.MeetingApp && k == "T":
			m.setFlash("meetings are learned by hand: a [[rule]] with bundle tock.meeting; t to relabel this one", true)
		case k == "t" && r.Label.CategorySource == classify.SourceLearned:
			// A learned label is a smart tag: fix the rule so every span
			// it labels changes, not only this row (o still tags just these).
			m.picker = newPicker(m.src, pickLearn, nil, r.Key, "").withQuick(&m, r.Key)
		case k == "t":
			m.picker = newPicker(m.src, pickTag, r.IDs, "", spansWhat(len(r.IDs), r.End-r.Start)).withQuick(&m, r.Key)
		case k == "p":
			m.picker = newPicker(m.src, pickProject, r.IDs, "", spansWhat(len(r.IDs), r.End-r.Start))
			m.picker.rowCategory = r.Label.Category
		default:
			m.picker = newPicker(m.src, pickLearn, nil, r.Key, "").withQuick(&m, r.Key)
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

// compact merges each run of labelled rows with one label (category and
// project), back to back, into one row: what the stretch was, not every
// window. Unlabelled, idle, away and meeting rows stay as they are, so
// each can still be labelled on its own. The merged row's app and text are
// those of its longest row; it holds every row's span ids.
func compact(rs []report.Row) []report.Row {
	out := make([]report.Row, 0, len(rs))
	longest := map[int]int64{} // out index -> its longest member's length
	for _, r := range rs {
		n := len(out)
		if n > 0 && mergeable(out[n-1]) && mergeable(r) && out[n-1].Label.Category == r.Label.Category &&
			out[n-1].Label.Project == r.Label.Project && r.Start-out[n-1].End < 60_000 {
			p := &out[n-1]
			if d := r.End - r.Start; d > longest[n-1] {
				p.App, p.Text, p.Key, longest[n-1] = r.App, r.Text, r.Key, d
			}
			p.IDs = append(append([]int64{}, p.IDs...), r.IDs...)
			p.End, p.Open = r.End, r.Open
			continue
		}
		longest[n] = r.End - r.Start
		out = append(out, r)
	}
	return out
}

func mergeable(r report.Row) bool {
	return !r.Idle && r.Label.Category != "" && r.App != span.AwayApp && r.App != span.MeetingApp && !remoteRow(&r)
}

// kindOf is the kind of category name in the day shown, "" when it has no
// time.
func (t *todayTab) kindOf(name string) string {
	for _, c := range t.day.Categories {
		if c.Name == name {
			return c.Kind
		}
	}
	return ""
}
