package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tock/internal/report"
)

// weekTab shows one Monday-start week.
type weekTab struct {
	date   time.Time // any day of the shown week; zero = this week
	week   report.Week
	loaded bool
	cursor int // 0 = Monday
}

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

func (t weekTab) view(w, h int) []string {
	if !t.loaded {
		return []string{"", " loading…"}
	}
	wk := t.week
	parts := []string{"tracked " + report.Dur(wk.TrackedMS), weekScore(wk)}
	for _, k := range report.Kinds {
		if ms := wk.ByKind[k]; ms > 0 {
			parts = append(parts, kindStyle(k).Render(kindName(k)+" "+report.Dur(ms)))
		}
	}
	out := []string{
		" " + sHead.Render("Week of "+wk.Monday.Format("Mon 2 Jan")) + "  " + strings.Join(parts, sDim.Render(" · ")),
		"",
	}
	var maxMS int64
	for _, d := range wk.Days {
		maxMS = max(maxMS, d.TrackedMS)
	}
	cells := max(min(w-32, 48), 8)
	for i, d := range wk.Days {
		dur, score, focus := report.Dur(d.TrackedMS), "—", "—"
		if d.Stats.ActiveMS > 0 {
			score, focus = fmt.Sprint(d.Score.Value), report.Dur(d.Stats.FocusMS)
		}
		bar := stacked(d.ByKind, maxMS, cells)
		if d.Future {
			dur, bar = "—", ""
		}
		line := fmt.Sprintf("%-3s %2d  ", d.Date.Format("Mon"), d.Date.Day()) + fitRight(dur, 5) + "  " +
			fitRight(score, 3) + "  " + fitRight(focus, 5) + "  " + bar
		if i == t.cursor {
			out = append(out, sSel.Render("▶"+line))
		} else {
			out = append(out, " "+line)
		}
	}
	out = append(out, "")
	var catMax int64
	for _, c := range wk.Categories {
		catMax = max(catMax, c.MS)
	}
	room := max(h-len(out)-2, 0)
	for _, c := range wk.Categories[:min(len(wk.Categories), room)] {
		bar := kindStyle(c.Kind).Render(report.Pad(report.Bar(c.MS, catMax, 12), 12))
		out = append(out, " "+fit(c.Name, 15)+fitRight(report.Dur(c.MS), 6)+"  "+bar+"  "+sDim.Render(projects(c.Projects)))
	}
	if len(wk.Projects) > 0 || len(wk.Apps) > 0 {
		out = append(out, "")
	}
	if len(wk.Projects) > 0 {
		out = append(out, " "+fit("Projects", 15)+"  "+names(wk.Projects))
	}
	if len(wk.Apps) > 0 {
		out = append(out, " "+fit("Apps", 15)+"  "+names(wk.Apps))
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

// spark is the sparkline cell of a score: ▁ for 0-12 up to █ for 88-100.
func spark(score int) rune { return []rune("▁▂▃▄▅▆▇█")[min(max(score, 0)*8/100, 7)] }

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
