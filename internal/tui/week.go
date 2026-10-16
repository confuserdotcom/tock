package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/classify"
	"tock/internal/report"
)

// weekTab shows one Monday-start week, or (m) one month's trends.
type weekTab struct {
	date   time.Time // any day of the shown week; zero = this week
	week   report.Week
	loaded bool
	cursor int // 0 = Monday

	month       bool      // month mode
	monthDate   time.Time // any day of the shown month; zero = this month
	monthData   report.Month
	monthLoaded bool
}

func (t *weekTab) monthOr(now time.Time) time.Time {
	if t.monthDate.IsZero() {
		return now
	}
	return t.monthDate
}

func (t *weekTab) setMonth(m report.Month) { t.monthData, t.monthLoaded = m, true }

func (t *weekTab) dateOr(now time.Time) time.Time {
	if t.date.IsZero() {
		return now
	}
	return t.date
}

func (t *weekTab) set(w report.Week) { t.week, t.loaded = w, true }

func (t *weekTab) clamp() { t.cursor = min(max(t.cursor, 0), 6) }

func (m Model) weekKey(k string) (tea.Model, tea.Cmd) {
	t := &m.week
	now := m.now()
	if k == "m" {
		t.month = !t.month
		return m, m.reload()
	}
	if t.month {
		return m.monthKey(k)
	}
	switch k {
	case "j", "down":
		t.cursor++
	case "k", "up":
		t.cursor--
	case "h":
		t.date, t.loaded = t.dateOr(now).AddDate(0, 0, -7), false
		return m, m.reload()
	case "l":
		next := t.dateOr(now).AddDate(0, 0, 7)
		if report.MondayOf(next).After(now) {
			return m, nil
		}
		if sameDay(report.MondayOf(next), report.MondayOf(now)) {
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
		if !t.loaded || t.week.Days[t.cursor].Future {
			return m, nil
		}
		d := t.week.Days[t.cursor].Date
		m.today = todayTab{}
		if !sameDay(d, now) {
			m.today.date = d
		}
		m.tab = tabToday
		return m, m.reload()
	}
	return m, nil
}

func (m Model) monthKey(k string) (tea.Model, tea.Cmd) {
	t := &m.week
	now := m.now()
	cur := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	shown := t.monthOr(now)
	shown = time.Date(shown.Year(), shown.Month(), 1, 0, 0, 0, 0, now.Location())
	switch k {
	case "h":
		t.monthDate, t.monthLoaded = shown.AddDate(0, -1, 0), false
	case "l":
		next := shown.AddDate(0, 1, 0)
		if next.After(cur) {
			return m, nil
		}
		if next.Equal(cur) {
			next = time.Time{}
		}
		t.monthDate, t.monthLoaded = next, false
	case ".":
		if t.monthDate.IsZero() {
			return m, nil
		}
		t.monthDate, t.monthLoaded = time.Time{}, false
	default:
		return m, nil
	}
	return m, m.reload()
}

// monthView renders the trends: report.MonthLines with the title in the
// heading style, labels muted, each category with its kind's dot and every
// change coloured by whether it is good news.
func (t weekTab) monthView(w int) []string {
	if !t.monthLoaded {
		return []string{"", center(sDim.Render("loading…"), w)}
	}
	lines := report.MonthLines(t.monthData)
	cats := t.monthData.Categories
	first := len(lines) - len(cats) // the category rows come last
	out := make([]string, len(lines))
	for i, l := range lines {
		switch c := i - first; {
		case i == 0:
			out[i] = " " + sHead.Render(l) + navArrows()
		case c >= 0 && len(cats) > 0:
			name := report.Pad(cats[c].Name, 16)
			kind := cats[c].Kind
			up := kind != classify.KindDistract && kind != ""
			out[i] = fit(" "+dot(kind)+" "+kindStyle(kind).Render(name)+colorChange(strings.TrimPrefix(l, name), up), w)
		case l == "":
			out[i] = ""
		default:
			label, rest, _ := strings.Cut(l, " ")
			out[i] = fit("   "+sDim.Render(label)+" "+colorChange(rest, true), w)
		}
	}
	return out
}

// colorChange colours the "+x"/"-x" changes in s: good when the change goes
// the way up says is good, else the accent.
func colorChange(s string, up bool) string {
	var b strings.Builder
	for _, f := range strings.SplitAfter(s, " ") {
		w := strings.TrimSpace(f)
		switch {
		case strings.HasPrefix(w, "+") && w != "+0%":
			if up {
				b.WriteString(sGood.Render(f))
			} else {
				b.WriteString(sAccent.Render(f))
			}
		case strings.HasPrefix(w, "-") || strings.HasPrefix(w, "−"):
			if up {
				b.WriteString(sAccent.Render(f))
			} else {
				b.WriteString(sGood.Render(f))
			}
		case strings.ContainsAny(w, "▁▂▃▄▅▆▇█"):
			b.WriteString(sBrand.Render(f))
		default:
			b.WriteString(sText.Render(f))
		}
	}
	return b.String()
}

// stacked draws a day's time by kind as one bar of up to cells blocks.
func stacked(byKind map[string]int64, maxMS int64, cells int) string {
	var b strings.Builder
	used := 0
	var acc int64
	for _, k := range report.Kinds {
		acc += byKind[k]
		upto := int(acc * int64(cells) / max(maxMS, 1))
		if byKind[k] > 0 && upto == used {
			upto++ // never hide a kind with time
		}
		upto = min(upto, cells)
		if upto > used {
			b.WriteString(kindStyle(k).Render(strings.Repeat("█", upto-used)))
			used = upto
		}
	}
	return b.String() + strings.Repeat(" ", cells-used)
}

// head is the number of lines above the day rows: the title, the plan when
// the week has one, and a blank line.
func (t *weekTab) head() int {
	if t.week.Plan != "" {
		return 4
	}
	return 3
}

// rowAt is the day (0 = Monday) shown on body line line, if any.
func (t *weekTab) rowAt(line int) (int, bool) {
	i := line - t.head()
	if !t.loaded || t.month || i < 0 || i >= len(t.week.Days) {
		return 0, false
	}
	return i, true
}

func (t weekTab) view(w, h int) []string {
	if !t.loaded {
		return []string{"", " loading…"}
	}
	wk := t.week
	parts := []string{sBright.Render(report.Dur(wk.TrackedMS)) + sDim.Render(" tracked"), weekScore(wk)}
	out := []string{spread(" "+sHead.Render("Week of "+wk.Monday.Format("Mon 2 Jan"))+navArrows(),
		strings.Join(parts, sDim.Render(" · "))+" ", w)}
	if wk.Plan != "" {
		out = append(out, fit(" "+sBrand.Render("◆ plan  ")+sText.Render(wk.Plan), w))
	}
	out = append(out, "", sFaint.Render("          tracked  score  focus"))
	var maxMS int64
	for _, d := range wk.Days {
		maxMS = max(maxMS, d.TrackedMS)
	}
	cells := max(min(w-32, 48), 8)
	for i, d := range wk.Days {
		dur, score, focus := report.Dur(d.TrackedMS), sFaint.Render("  ·"), sFaint.Render("    ·")
		if d.Stats.ActiveMS > 0 {
			band, _ := scoreBand(d.Score.Value, true)
			score, focus = band.Bold(true).Render(fitRight(fmt.Sprint(d.Score.Value), 3)), sText.Render(fitRight(report.Dur(d.Stats.FocusMS), 5))
		}
		bar := stacked(d.ByKind, maxMS, cells)
		day := sText.Render(fmt.Sprintf("%-3s %2d", d.Date.Format("Mon"), d.Date.Day()))
		durS := sBright.Render(fitRight(dur, 6))
		switch {
		case d.Future:
			day, durS, bar = sFaint.Render(fmt.Sprintf("%-3s %2d", d.Date.Format("Mon"), d.Date.Day())), sFaint.Render(fitRight("·", 6)), ""
		case d.TrackedMS == 0:
			durS, bar = sFaint.Render(fitRight("·", 6)), ""
		}
		line := day + "  " + durS + "    " + score + "  " + focus + "  " + bar
		if i == t.cursor {
			out = append(out, selRow(sBrand.Bold(true).Render("▸")+line, w-1))
		} else {
			out = append(out, " "+line)
		}
	}
	out = append(out, "")
	if len(wk.Goals) > 0 {
		for _, g := range wk.Goals {
			st, mark := sText, "○ "
			switch {
			case g.State == report.GoalOver || (!g.Goal.Week && g.Goal.AtMost && g.Held < g.Days):
				st, mark = sAccent, "✗ "
			case g.State == report.GoalMet || g.State == report.GoalOK || (!g.Goal.Week && g.Days > 0 && g.Held == g.Days):
				st, mark = sGood, "✓ "
			}
			out = append(out, " "+st.Render(mark)+fit(g.Goal.Text(), 26)+"  "+st.Render(report.WeekGoalText(g)))
		}
		out = append(out, "")
	}
	var catMax int64
	for _, c := range wk.Categories {
		catMax = max(catMax, c.MS)
	}
	mods := moduleLines(wk.Modules, w)
	room := max(h-len(out)-2-len(mods), 0)
	for _, c := range wk.Categories[:min(len(wk.Categories), room)] {
		name := c.Name
		if name == report.Unclassified {
			name = "unlabelled"
		}
		out = append(out, " "+dot(c.Kind)+" "+kindStyle(c.Kind).Render(fit(name, 14))+sBright.Render(fitRight(report.Dur(c.MS), 6))+
			"  "+meter(c.MS, catMax, 12, kindStyle(c.Kind))+"  "+sDim.Render(projects(c.Projects)))
	}
	out = append(out, mods...)
	if len(wk.Projects) > 0 || len(wk.Apps) > 0 {
		out = append(out, "")
	}
	if len(wk.Projects) > 0 {
		out = append(out, " "+sDim.Render(fit("Projects", 16))+"  "+names(wk.Projects))
	}
	if len(wk.Commits) > 0 {
		out = append(out, " "+sDim.Render(fit("Commits", 16))+"  "+report.CommitsLine(wk.Commits))
	}
	if len(wk.Apps) > 0 {
		out = append(out, " "+sDim.Render(fit("Apps", 16))+"  "+names(wk.Apps))
	}
	return out
}

// moduleLines is the week's modules under their categories, each bar out
// of its weekly goal when it has one: "Study  Analyse 1  4h12 ━━━━── 6h ✓".
func moduleLines(mods []report.ModuleRow, w int) []string {
	if len(mods) == 0 {
		return nil
	}
	var scale int64
	for _, m := range mods {
		scale = max(scale, m.MS, m.GoalMS)
	}
	barW := max(min(w-52, 24), 8)
	out := []string{"", " " + sHead.Render("Modules")}
	last := ""
	for _, m := range mods {
		st := kindStyle(m.Kind)
		cat := ""
		if m.Category != last {
			cat, last = m.Category, m.Category
		}
		goal := ""
		total := scale
		if m.GoalMS > 0 {
			total = m.GoalMS
			switch {
			case m.MS >= m.GoalMS:
				goal = sGood.Render("✓ " + report.Dur(m.GoalMS))
			default:
				goal = sDim.Render("of " + report.Dur(m.GoalMS) + " · " + report.Dur(m.GoalMS-m.MS) + " to go")
			}
		}
		line := " " + st.Render(fit(report.Safe(cat), 14)) + sText.Render(fit(report.Safe(m.Name), 18)) +
			sBright.Render(fitRight(dashDur(m.MS), 6)) + "  " + meter(m.MS, total, barW, st) + "  " + goal
		out = append(out, z("mod:"+m.Name, fit(line, w)))
	}
	return out
}

func names(list []report.NameMS) string {
	var ps []string
	for _, p := range list {
		ps = append(ps, report.Safe(p.Name)+" "+report.Dur(p.MS))
	}
	return strings.Join(ps, " · ")
}

// spark is the sparkline cell of a score.
func spark(score int) rune { return report.SparkCell(score) }

// weekScore is "avg score 64 ▃▅▇▆▂· " over the days with activity, with one
// sparkline cell per day: · for no activity, space for the future.
func weekScore(wk report.Week) string {
	var b strings.Builder
	sum, n := 0, 0
	for _, d := range wk.Days {
		switch {
		case d.Future:
			b.WriteRune(' ')
		case d.Stats.ActiveMS <= 0:
			b.WriteRune('·')
		default:
			b.WriteRune(spark(d.Score.Value))
			sum += d.Score.Value
			n++
		}
	}
	avg := "—"
	if n > 0 {
		avg = fmt.Sprint((sum + n/2) / n)
	}
	return sText.Render("avg score "+avg+" ") + sAccent.Render(strings.TrimRight(b.String(), " "))
}
