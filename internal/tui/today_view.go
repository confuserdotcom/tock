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

// sidebarMin is the narrowest body with the sidebar: Where it went, goals
// and work blocks on the left, the day on the right. sidebarW is its width.
const sidebarMin, sidebarW = 130, 38

// split is the main column's width and the sidebar's (0 without one).
func split(w int) (main, side int) {
	if w >= sidebarMin {
		return w - sidebarW - 1, sidebarW
	}
	return w, 0
}

// head is everything above the timeline on a body w×h, ending with a blank
// line. The view, the scroll and the mouse all measure it, so they agree.
//
// w is the body's width: with a sidebar, the head is the main column's,
// and the cards and the goal and work lines are in the sidebar.
func (t *todayTab) head(w, h int, now time.Time) []string {
	w, side := split(w)
	out := t.hero(w, now)
	if h >= 24 || side > 0 {
		out = append(out, ribbon(t.day, w, t.day.Date.Location())...)
	}
	if side > 0 {
		if a := t.attention(); a != "" {
			out = append(out, fit(a, w))
		}
		if t.day.Summary != "" {
			out = append(out, fit(" "+sBrand.Render("✎ ")+sText.Render(report.Safe(t.day.Summary)), w))
		}
		return append(out, "")
	}
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

// rowW is the timeline's width in a body w wide.
func rowW(w int) int {
	mw, _ := split(w)
	return mw
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
	switch {
	case d.Off:
		word = "day off"
	case !active && d.Work.Set && d.TrackedMS > 0:
		word = "outside hours"
	}
	num := "–"
	if active {
		num = fmt.Sprint(sc.Value)
	}
	left := " " + z("key:i", band.Bold(true).Render(num)+"  "+sDim.Render("score")+"  "+band.Render(word))
	if d.Work.Set && !d.Off {
		hours := "hours " + report.HoursText(d)
		if d.OutsideMS >= 60_000 {
			hours += " +" + report.Dur(d.OutsideMS) + " outside"
		}
		left += sFaint.Render("  · ") + sDim.Render(hours)
	}
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
	line2 := spread(" "+sHead.Render(title)+navArrows(), sDim.Render(strings.Join(facts, " · "))+" ", w)
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
		out = append(out, fit(" "+z("tab:1", goalChips(d.Goals)), w))
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
		parts[i] = z(fmt.Sprintf("at:%d", b.Start), sDim.Render(time.UnixMilli(b.Start).In(loc).Format("15:04")+" ")+
			kindStyle(b.Kind).Render(report.Safe(b.Name))+" "+sBright.Render(report.Dur(b.ActiveMS)))
	}
	return strings.Join(parts, sFaint.Render(" · "))
}

// attention is the one thing that wants an answer: pending time away, else
// unlabelled time waiting in the inbox.
func (t *todayTab) attention() string {
	d := t.day
	if n := len(d.Away); n > 0 {
		iv := d.Away[n-1]
		line := " " + sAccent.Bold(true).Render("⚑ Away "+awayWhat(iv, d.Date.Location()))
		if b := t.before(iv); b.Category != "" {
			line += sText.Render(" after ") + kindStyle(t.kindOf(b.Category)).Render(report.Safe(report.LabelText(b.Category, b.Project))) +
				sText.Render(" — still on it?   ") + keycap("y", "yes") + "   " + keycap("a", "something else") + "   " + keycap("A", "leave idle")
		} else {
			line += sText.Render(" — what was it?   ") + keycap("a", "label") + "   " + keycap("A", "leave idle")
		}
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
			at := time.Date(d.Date.Year(), d.Date.Month(), d.Date.Day(), h, 30, 0, 0, d.Date.Location()).UnixMilli()
			b.WriteString(tip(fmt.Sprintf("at:%d", at), fmt.Sprintf("%02d:00  %s %s", h, report.Dur(hours[h]), kindName(k)),
				strings.Repeat(shade(hours[h]), cell)))
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
			"  " + meter(c.MS, maxMS, mw, kindStyle(c.Kind)) + "  " + projectChips(c, extra)
		filter := c.Name
		if c.Name == report.Unclassified {
			filter = "—"
		}
		out = append(out, tip("cat:"+filter, "show only "+name+" rows · click again for all", fit(line, w)))
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
	mw, side := split(w)
	out := t.head(w, h, now)
	rs := t.rows()
	end := min(t.top+t.layout(w, h), len(rs))
	for i := t.top; i < end; i++ {
		out = append(out, t.timelineRow(i, rs[i], mw, i == t.cursor, now.Location()))
	}
	if len(rs) == 0 && t.filter != "" {
		out = append(out, sDim.Render(fmt.Sprintf("   no rows match %q · ", t.filter))+keycap("esc", "show all"))
	}
	if side == 0 {
		return out
	}
	return sideBySide(fitBody(t.sidebar(side, h), h), fitBody(out, h), side, 1)
}

// sidebar is Where it went, with each category's modules, then the goals,
// the commits and the work blocks, h lines at most, w wide. Everything in
// it is a button that filters or jumps the timeline.
func (t todayTab) sidebar(w, h int) []string {
	d := t.day
	inner := w - 4
	var where []string
	var maxMS int64
	for _, c := range d.Categories {
		maxMS = max(maxMS, c.MS)
	}
	for _, c := range d.Categories {
		name, filter := c.Name, c.Name
		if c.Name == report.Unclassified {
			name, filter = "unlabelled", "—"
		}
		st := kindStyle(c.Kind)
		on := t.filter != "" && t.filter == filter
		mark := dot(c.Kind) + " "
		if on {
			mark = sBrand.Bold(true).Render("▸ ")
		}
		head := mark + st.Bold(on).Render(fit(report.Safe(name), inner-8)) + sBright.Render(fitRight(report.Dur(c.MS), 6))
		where = append(where, tip("cat:"+filter, "show only "+name+" rows · click again for all", fit(head, inner)),
			"  "+meter(c.MS, maxMS, inner-2, st))
		if c.Name == report.Unclassified && len(c.Items) > 0 {
			where = append(where, "  "+sDim.Render(plural(len(c.Items), "key")+" · ")+keycap("3", "inbox"))
		}
		for _, p := range c.Projects {
			f := report.LabelText(c.Name, p.Name)
			where = append(where, tip("cat:"+f, "show only "+p.Name+" rows", fit("  "+sFaint.Render("╰ ")+
				sText.Render(fit(report.Safe(p.Name), inner-10))+sDim.Render(fitRight(report.Dur(p.MS), 6)), inner)))
		}
	}
	out := card("Where it went", where, w)
	if len(d.Goals) > 0 {
		var g []string
		for _, r := range d.Goals {
			g = append(g, z("tab:1", goalChips([]report.GoalRow{r})))
		}
		out = append(out, card("Goals", g, w)...)
	}
	if len(d.Blocks) > 0 {
		var bl []string
		loc := d.Date.Location()
		for _, b := range d.Blocks {
			line := sDim.Render(time.UnixMilli(b.Start).In(loc).Format("15:04")+" ") +
				kindStyle(b.Kind).Render(fit(report.Safe(b.Name), inner-13)) + sBright.Render(fitRight(report.Dur(b.ActiveMS), 6))
			bl = append(bl, tip(fmt.Sprintf("at:%d", b.Start), "jump to "+time.UnixMilli(b.Start).In(loc).Format("15:04"), fit(line, inner)))
		}
		out = append(out, card("Work blocks", bl, w)...)
	}
	if len(d.Commits) > 0 {
		out = append(out, card("Commits", []string{sText.Render(report.CommitsLine(d.Commits))}, w)...)
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
func (t todayTab) timelineRow(i int, r report.Row, w int, sel bool, loc *time.Location) string {
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
	line = z("row:"+fmt.Sprint(i), fit(line, w))
	if sel {
		return selRow(line, w)
	}
	return line
}

// projectChips are a category's projects (or its inbox keys), each a
// button that shows only its rows.
func projectChips(c report.CatRow, extra string) string {
	if c.Name == report.Unclassified || len(c.Projects) == 0 {
		return sDim.Render(extra)
	}
	var parts []string
	for _, p := range c.Projects[:min(len(c.Projects), 3)] {
		parts = append(parts, tip("cat:"+report.LabelText(c.Name, p.Name), "show only "+p.Name+" rows",
			sDim.Render(report.Safe(p.Name)+" "+report.Dur(p.MS))))
	}
	if n := len(c.Projects) - 3; n > 0 {
		parts = append(parts, sFaint.Render(fmt.Sprintf("+%d", n)))
	}
	return strings.Join(parts, sFaint.Render(" · "))
}
