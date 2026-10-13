package report

import (
	"fmt"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/span"
)

// Goal states.
const (
	GoalMet   = "met"   // an at_least goal reached its target
	GoalShort = "short" // an at_least goal has not yet
	GoalOK    = "ok"    // a limit not passed
	GoalOver  = "over"  // a limit passed
)

// GoalRow is a goal's progress on one day: the day for a day goal, the
// week up to the end of the day for a week goal.
type GoalRow struct {
	Goal         classify.Goal
	DoneMS       int64
	State        string
	Streak, Best int // filled by the caller; see Streak
}

// goalMS is the non-idle time in [from, to) that counts towards g.
func goalMS(g classify.Goal, views []classify.SpanView, labels []classify.Label, from, to int64) int64 {
	var ms int64
	for i, v := range views {
		if v.BundleID == span.IdleBundleID || !g.Matches(labels[i]) {
			continue
		}
		ms += max(min(v.End, to)-max(v.Start, from), 0)
	}
	return ms
}

func goalState(g classify.Goal, done int64) string {
	switch {
	case g.AtMost && done > g.MS:
		return GoalOver
	case g.AtMost:
		return GoalOK
	case done >= g.MS:
		return GoalMet
	}
	return GoalShort
}

// Goals is the progress of each goal on day (local midnight), from the
// views of its Monday-start week up to the end of day.
func Goals(goals []classify.Goal, views []classify.SpanView, labels []classify.Label, day, monday time.Time) []GoalRow {
	out := make([]GoalRow, len(goals))
	end := day.AddDate(0, 0, 1).UnixMilli()
	for i, g := range goals {
		from := day.UnixMilli()
		if g.Week {
			from = monday.UnixMilli()
		}
		done := goalMS(g, views, labels, from, end)
		out[i] = GoalRow{Goal: g, DoneMS: done, State: goalState(g, done)}
	}
	return out
}

// GoalText renders one goal's progress: "Study 1h10/2h00",
// "project tock 6h20/10h00 wk", "Entertainment 50m/45m over".
func GoalText(r GoalRow) string {
	s := r.Goal.Name() + " " + Dur(r.DoneMS) + "/" + Dur(r.Goal.MS)
	if r.Goal.Week {
		s += " wk"
	}
	if r.State == GoalOver {
		s += " over"
	}
	return s + streakText(r.Goal, r.Streak)
}

// GoalsLine joins every goal's GoalText.
func GoalsLine(rows []GoalRow) string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = GoalText(r)
	}
	return strings.Join(parts, " · ")
}

// GoalsHeld counts the goals that are met or not over, and names the first
// limit that is over ("" when none is).
func GoalsHeld(rows []GoalRow) (held int, over string) {
	for _, r := range rows {
		switch r.State {
		case GoalMet, GoalOK:
			held++
		case GoalOver:
			if over == "" {
				over = r.Goal.Name()
			}
		}
	}
	return held, over
}

// WeekGoalRow is a goal over a week. A week goal has its total; a day goal
// counts the active days (past days and today with active time) and those
// it held on (met, or not over).
type WeekGoalRow struct {
	Goal         classify.Goal
	DoneMS       int64 // week goals
	State        string
	Days, Held   int // day goals
	Streak, Best int // filled by the caller; see Streak
}

// WeekGoals is each goal over the Monday-start week of monday, up to nowMS.
func WeekGoals(goals []classify.Goal, views []classify.SpanView, labels []classify.Label, monday time.Time, nowMS int64) []WeekGoalRow {
	out := make([]WeekGoalRow, len(goals))
	days := splitDays(monday, views, labels)
	for i, g := range goals {
		r := WeekGoalRow{Goal: g}
		if g.Week {
			r.DoneMS = goalMS(g, views, labels, monday.UnixMilli(), monday.AddDate(0, 0, 7).UnixMilli())
			r.State = goalState(g, r.DoneMS)
			out[i] = r
			continue
		}
		for d := range days {
			date := monday.AddDate(0, 0, d)
			if date.UnixMilli() > nowMS || !active(days[d].views) {
				continue
			}
			r.Days++
			if st := goalState(g, goalMS(g, days[d].views, days[d].labels, date.UnixMilli(), date.AddDate(0, 0, 1).UnixMilli())); st != GoalOver && st != GoalShort {
				r.Held++
			}
		}
		out[i] = r
	}
	return out
}

func active(vs []classify.SpanView) bool {
	for _, v := range vs {
		if v.BundleID != span.IdleBundleID {
			return true
		}
	}
	return false
}

// WeekGoalText renders a goal over a week: "6h20/10h00 short 3h40",
// "12h00/10h00 met", "3h00/5h00 ok", "6h00/5h00 over 1h00" for week goals;
// "met 3 of 5 days" or "over 2 of 5 days" for day goals.
func WeekGoalText(r WeekGoalRow) string {
	s := weekGoalText(r)
	if r.Best >= 2 {
		s += ", streak " + streakUnit(r.Goal, r.Streak) + " (best " + streakUnit(r.Goal, r.Best) + ")"
	}
	return s
}

func weekGoalText(r WeekGoalRow) string {
	g := r.Goal
	if g.Week {
		s := Dur(r.DoneMS) + "/" + Dur(g.MS)
		switch r.State {
		case GoalShort:
			return s + " short " + Dur(g.MS-r.DoneMS)
		case GoalOver:
			return s + " over " + Dur(r.DoneMS-g.MS)
		}
		return s + " " + r.State
	}
	if r.Days == 0 {
		return "no active days"
	}
	if g.AtMost {
		return fmt.Sprintf("over %d of %d %s", r.Days-r.Held, r.Days, dayWord(r.Days))
	}
	return fmt.Sprintf("met %d of %d %s", r.Held, r.Days, dayWord(r.Days))
}

func dayWord(n int) string {
	if n == 1 {
		return "day"
	}
	return "days"
}
