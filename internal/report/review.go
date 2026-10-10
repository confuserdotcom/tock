package report

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"tock/internal/stats"
)

// PlanMaxRunes is the longest plan tock review keeps.
const PlanMaxRunes = 200

// CleanPlan trims a typed plan, replaces control characters and cuts it to
// PlanMaxRunes.
func CleanPlan(s string) string {
	s = strings.TrimSpace(Safe(strings.TrimSpace(s)))
	if utf8.RuneCountInString(s) > PlanMaxRunes {
		s = string([]rune(s)[:PlanMaxRunes])
	}
	return s
}

// ReviewLines renders the weekly review of w:
//
//	Week of Mon 5 Oct
//	tracked 31h20 · focus 14h10 · score 66 avg · 5 workdays · 6.1 switches/h
//	plan: finish the ATLAS report
//
//	goals
//	  Study ≥2h/day                 met 3 of 5 days
//
//	top distractions
//	  youtube.com        2h10
//
//	best day  Wed 7 Oct · score 81
func ReviewLines(w Week) []string {
	days := make([]MonthDay, 0, 7)
	for _, d := range w.Days {
		if !d.Future {
			days = append(days, MonthDay{Stats: d.Stats, Score: d.Score})
		}
	}
	s, _ := side(nil, days)
	head := []string{"tracked " + Dur(w.TrackedMS), "focus " + Dur(s.FocusMS)}
	if s.Workdays > 0 {
		head = append(head, fmt.Sprintf("score %d avg", int(math.Round(s.AvgScore))),
			fmt.Sprintf("%d %s", s.Workdays, plural(s.Workdays, "workday", "workdays")),
			fmt.Sprintf("%.1f switches/h", s.SwitchesPerHour))
	} else {
		head = append(head, "no workdays")
	}
	out := []string{"Week of " + w.Monday.Format("Mon 2 Jan"), strings.Join(head, " · ")}
	if w.Plan != "" {
		out = append(out, "plan: "+w.Plan)
	}
	if len(w.Goals) > 0 {
		out = append(out, "", "goals")
		for _, g := range w.Goals {
			out = append(out, "  "+Pad(g.Goal.Text(), 28)+"  "+WeekGoalText(g))
		}
	}
	out = append(out, "")
	if len(w.Distractions) == 0 {
		out = append(out, "top distractions  none")
	} else {
		out = append(out, "top distractions")
		for _, d := range w.Distractions {
			out = append(out, fmt.Sprintf("  %s %6s", Pad(Safe(d.Name), 16), Dur(d.MS)))
		}
	}
	best := -1
	for i, d := range w.Days {
		if !d.Future && d.Stats.ActiveMS >= stats.WorkdayMinMS && (best < 0 || d.Score.Value > w.Days[best].Score.Value) {
			best = i
		}
	}
	if best >= 0 {
		out = append(out, "", fmt.Sprintf("best day  %s · score %d", w.Days[best].Date.Format("Mon 2 Jan"), w.Days[best].Score.Value))
	}
	return out
}
