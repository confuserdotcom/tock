package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"tock/internal/report"
	"tock/internal/span"
)

// minTimeline is the fewest timeline rows the today tab keeps.
const minTimeline = 4

// sideBySideMin is the narrowest body that shows the two cards side by side.
const sideBySideMin = 96

// head is everything above the timeline on a body w×h, ending with a blank
// line. The view, the scroll and the mouse all measure it, so they agree.
func (t *todayTab) head(w, h int, now time.Time) []string {
	out := t.hero(w, now)
	out = append(out, t.notes(w)...)
	out = append(out, "")
	room := h - len(out) - minTimeline - 1 // the blank line under the cards
	out = append(out, t.cards(w, room)...)
	return append(out, "")
}

// layout is how many timeline rows fit in a body w×h.
func (t *todayTab) layout(w, h int) int {
	return max(h-len(t.head(w, h, time.Time{})), 1)
}

// rowAt is the timeline row shown on body line line of a body w×h, if any.
func (t *todayTab) rowAt(line, w, h int) (int, bool) {
	if !t.loaded || len(t.day.Timeline) == 0 && t.filter == "" {
		return 0, false
	}
	start := len(t.head(w, h, time.Time{}))
	i := t.top + line - start
	if line < start || line >= start+t.layout(w, h) || i >= len(t.rows()) {
		return 0, false
	}
	return i, true
}

// hero is the score, the focus meter and the day's numbers: two lines.
func (t *todayTab) hero(w int, now time.Time) []string {
	d := t.day
	st, sc := d.Stats, d.Score
	active := st.ActiveMS > 0
	band, word := scoreBand(sc.Value, active)
	num := "–"
	if active {
		num = fmt.Sprint(sc.Value)
	}
	left := " " + band.Bold(true).Render(num) + "  " + sDim.Render("score") + "  " + band.Render(word)
	mw := min(24, max(w/5, 8))
	right := sBright.Render(report.Dur(st.FocusMS)) + sDim.Render(" focus of "+report.Dur(sc.Target.MS)) +
		"  " + meter(st.FocusMS, sc.Target.MS, mw, sBrand) + " "
	line1 := spread(left, right, w)

	title := "Today"
	if !t.isToday(now) {
		title = d.Date.Format("Mon 2 Jan")
	}
	var facts []string
	if n := len(st.Blocks); n > 0 {
		facts = append(facts, fmt.Sprintf("%s · longest %s", plural(n, "block"), report.Dur(st.LongestMS)))
	}
	if active {
		facts = append(facts, plural2(st.Switches, "switch", "switches"))
	}
	facts = append(facts, "tracked "+report.Dur(d.TrackedMS), "idle "+report.Dur(d.IdleMS))
	line2 := spread(" "+sHead.Render(title)+sFaint.Render("  ‹ h  l ›"), sDim.Render(strings.Join(facts, " · "))+" ", w)
	return []string{line1, line2}
}

func plural2(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// notes are the goal chips, the commits and one attention line.
func (t *todayTab) notes(w int) []string {
	d := t.day
	var out []string
	if len(d.Goals) > 0 {
		out = append(out, fit(" "+goalChips(d.Goals), w))
	}
	if len(d.Commits) > 0 {
		out = append(out, fit(" "+sDim.Render("commits  ")+sText.Render(report.CommitsLine(d.Commits)), w))
	}
	if len(d.Blocks) > 0 {
		out = append(out, fit(" "+sDim.Render("work  ")+workLine(d.Blocks, d.Date.Location()), w))
	}
	if d.Summary != "" {
		out = append(out, fit(" "+sBrand.Render("✎ ")+sText.Render(report.Safe(d.Summary)), w))
	}
	if a := t.attention(); a != "" {
		out = append(out, fit(a, w))
	}
	return out
}

// workLine lists the work blocks as "09:00 tock 1h40 · 11:05 Study 50m".
func workLine(blocks []report.Block, loc *time.Location) string {
	parts := make([]string, len(blocks))
	for i, b := range blocks {
		parts[i] = sDim.Render(time.UnixMilli(b.Start).In(loc).Format("15:04")+" ") +
			kindStyle(b.Kind).Render(report.Safe(b.Name)) + " " + sBright.Render(report.Dur(b.ActiveMS))
	}
	return strings.Join(parts, sFaint.Render(" · "))
}

// attention is the one thing that wants an answer: pending time away, else
// unlabelled time waiting in the inbox.
func (t *todayTab) attention() string {
	d := t.day
	if n := len(d.Away); n > 0 {
		iv := d.Away[n-1]
		line := " " + sAccent.Bold(true).Render("⚑ Away "+awayWhat(iv, d.Date.Location())) +
			sText.Render(" — what was it?   ") + keycap("a", "label") + "   " + keycap("A", "leave idle")
		if n > 1 {
			line += sDim.Render(fmt.Sprintf("   +%d more", n-1))
		}
		return line
	}
	for _, c := range d.Categories {
		if c.Name == report.Unclassified && len(c.Items) > 0 {
			return " " + sUnl.Render("◌ "+report.Dur(c.MS)+" unlabelled") + sDim.Render(" · "+plural(len(c.Items), "key")+" in the ") +
				keycap("3", "inbox") + sDim.Render(" · or ") + keycap("enter", "on a row")
		}
	}
	return ""
}

// goalChips renders each goal as "✓ Study 2h/2h", "○ …" or "✗ …".
func goalChips(rows []report.GoalRow) string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		text := report.GoalText(r)
		switch r.State {
		case report.GoalMet, report.GoalOK:
			parts[i] = sGood.Render("✓ " + text)
		case report.GoalOver:
			parts[i] = sAccent.Render("✗ " + text)
		default:
			parts[i] = sText.Render("○ " + text)
		}
	}
	return strings.Join(parts, "   ")
}

// cards are the Day and Where-it-went cards, side by side on a wide body,
// else stacked, in at most room lines. When the Day card does not fit, only
// Where it went shows; when that does not fit either, nothing does.
func (t *todayTab) cards(w, room int) []string {
	d := t.day
	if len(d.Categories) == 0 || room < 3 {
		return nil
	}
	if w >= sideBySideMin {
		cw := (w - 3) / 2
		heat := heatLines(d, cw-4)
		if room >= len(heat)+2 {
			cats := catLines(d, w-3-cw-4, room-2)
			for len(heat) < len(cats) {
				heat = append(heat, "")
			}
			for len(cats) < len(heat) {
				cats = append(cats, "")
			}
			return sideBySide(prefix(card("Day", heat, cw), " "), card("Where it went", cats, w-3-cw), cw+1, 1)
		}
	} else if heat := heatLines(d, w-6); room >= len(heat)+2+3 {
		day := card("Day", heat, w-2)
		return prefix(append(day, card("Where it went", catLines(d, w-6, room-len(day)-2), w-2)...), " ")
	}
	return prefix(card("Where it went", catLines(d, w-6, room-2), w-2), " ")
}

func prefix(lines []string, p string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = p + l
	}
	return out
}

// heatLines is the hour axis and one row per kind, fitted to w cells.
func heatLines(d report.Day, w int) []string {
	const label = 11
	hrs := d.Hours
	n := hrs.Last - hrs.First + 1
	cell := min(max((w-label)/max(n, 1), 1), 4)
	step := 1
	for step*cell < 3 {
		step++
	}
	axis := []byte(strings.Repeat(" ", n*cell+2))
	for h := hrs.First; h <= hrs.Last; h += step {
		copy(axis[(h-hrs.First)*cell:], fmt.Sprintf("%02d", h))
	}
	out := []string{strings.Repeat(" ", label) + sFaint.Render(strings.TrimRight(string(axis), " "))}
	for _, k := range report.Kinds {
		hours, ok := hrs.MS[k]
		if !ok {
			continue
		}
		var b strings.Builder
		for h := hrs.First; h <= hrs.Last; h++ {
			b.WriteString(strings.Repeat(shade(hours[h]), cell))
		}
		st := kindStyle(k)
		out = append(out, st.Render(fit(kindName(k), label))+st.Render(b.String()))
	}
	return out
}

// catLines is one row per category, at most n, fitted to w cells.
func catLines(d report.Day, w, n int) []string {
	var maxMS int64
	for _, c := range d.Categories {
		maxMS = max(maxMS, c.MS)
	}
	nameW := 14
	mw := max(min(w-nameW-2-5-2-14, 10), 4)
	var out []string
	for i, c := range d.Categories {
		if i == n-1 && len(d.Categories) > n {
			out = append(out, sDim.Render(fmt.Sprintf("  +%d more", len(d.Categories)-i)))
			break
		}
		name, extra := c.Name, projects(c.Projects)
		if c.Name == report.Unclassified {
			name = "unlabelled"
			if len(c.Items) > 0 {
				extra = plural(len(c.Items), "key")
			}
		}
		line := dot(c.Kind) + " " + kindStyle(c.Kind).Render(fit(name, nameW)) + sBright.Render(fitRight(report.Dur(c.MS), 5)) +
			"  " + meter(c.MS, maxMS, mw, kindStyle(c.Kind)) + "  " + sDim.Render(extra)
		out = append(out, fit(line, w))
	}
	return out
}

// view renders the tab body in w×h cells.
func (t todayTab) view(w, h int, now time.Time) []string {
	if !t.loaded {
		return []string{"", center(sDim.Render("loading…"), w)}
	}
	if len(t.day.Timeline) == 0 {
		return t.empty(w, now)
	}
	out := t.head(w, h, now)
	rs := t.rows()
	end := min(t.top+t.layout(w, h), len(rs))
	for i := t.top; i < end; i++ {
		out = append(out, t.timelineRow(rs[i], w, i == t.cursor, now.Location()))
	}
	if len(rs) == 0 && t.filter != "" {
		out = append(out, sDim.Render(fmt.Sprintf("   no rows match %q · esc clears the filter", t.filter)))
	}
	return out
}

// empty is the today tab of a day with nothing tracked.
func (t todayTab) empty(w int, now time.Time) []string {
	if !t.isToday(now) {
		return emptyState(w, t.day.Date.Format("Mon 2 Jan"),
			sText.Render("Nothing tracked on this day."),
			"", keycap("l", "next day")+"   "+keycap(".", "back to today"))
	}
	if t.trackdUp {
		return emptyState(w, "Today",
			sText.Render("Nothing tracked yet today."),
			sDim.Render("trackd is recording: switch to an app and"),
			sDim.Render("it shows up here within a few seconds."),
			"", keycap("h", "yesterday")+"   "+keycap("?", "keys"))
	}
	return emptyState(w, "Today",
		sText.Render("Nothing tracked yet today."),
		sAccent.Render("trackd is not running."),
		sDim.Render("Start it with make install, or check tock doctor."),
		"", keycap("r", "retry")+"   "+keycap("h", "yesterday"))
}

// timelineRow is one timeline row: times, the label with its dot, the app
// and the text; an unlabelled row with a suggestion ends with it.
func (t todayTab) timelineRow(r report.Row, w int, sel bool, loc *time.Location) string {
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	mark := "  "
	if sel {
		mark = sBrand.Bold(true).Render("▸ ")
	}
	const labelW = 20
	var label string
	switch {
	case r.Idle:
		label = sFaint.Render(fit("  idle", labelW))
	case r.App == span.AwayApp || r.App == span.MeetingApp || r.Label.Category != "" || r.Label.Project != "":
		label = dot(r.Label.Kind) + " " + kindStyle(r.Label.Kind).Render(fit(rowLabel(r), labelW-2))
	default:
		label = dot("") + " " + sUnl.Render(fit("unlabelled", labelW-2))
	}
	tail := ""
	if want, ok := t.sugg[r.Key]; ok && !r.Idle && r.Label.Category == "" {
		tail = sUnl.Render("→ "+report.Safe(report.LabelText(want.Category, want.Project))) + " " + sBrand.Render("⏎")
	}
	fixed := 2 + 11 + 2 + 5 + 2 + labelW + 2 + 10 + 2
	textW := max(w-fixed-ansi.StringWidth(tail)-1, 0)
	text := sDim.Render(fit(r.Text, textW))
	times := sText.Render(clock(r.Start)) + sFaint.Render("–") + sText.Render(clock(r.End))
	line := mark + times + "  " + sBright.Render(fitRight(report.Dur(r.End-r.Start), 5)) + "  " + label + "  " +
		sText.Render(fit(r.App, 10)) + "  " + text
	if tail != "" {
		line += " " + tail
	}
	if sel {
		return selRow(line, w)
	}
	return line
}
