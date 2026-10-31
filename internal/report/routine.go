package report

import (
	"fmt"
	"strings"

	"tock/internal/plan"
)

// The start-of-day notice lists at most RoutineItems timed plan items, each
// cut to RoutineItemCells cells.
const (
	RoutineItems     = 3
	RoutineItemCells = 24
)

// StartBody is the start-of-day text: yesterday's result, then today's
// timed plan items.
// "Yesterday: 6h12 tracked, score 74, goals 2/3 met. Today: 10:00 Standup."
// today is plan.Rows for today: only the rows of the Today section with a
// time are listed, in the order given.
func StartBody(y Day, today []plan.Row) string {
	s := yesterdayText(y)
	var items []string
	for _, r := range today {
		if r.Section != plan.Today {
			continue
		}
		if m, ok := plan.Clock(r.Item); ok {
			items = append(items, fmt.Sprintf("%02d:%02d %s", m/60, m%60, Cut(r.Text, RoutineItemCells)))
		}
	}
	if len(items) == 0 {
		return s
	}
	if len(items) > RoutineItems {
		more := fmt.Sprintf("+%d more", len(items)-RoutineItems)
		items = append(items[:RoutineItems], more)
	}
	return s + " Today: " + strings.Join(items, " · ") + "."
}

// yesterdayText is the "Yesterday: …" sentence.
func yesterdayText(y Day) string {
	switch {
	case y.Off:
		return "Yesterday: day off."
	case y.Stats.ActiveMS <= 0 && y.Work.Set && y.TrackedMS > 0:
		return "Yesterday: no activity in work hours."
	case y.Stats.ActiveMS <= 0:
		return "Yesterday: no activity."
	}
	s := fmt.Sprintf("Yesterday: %s tracked, score %d", Dur(y.Stats.ActiveMS), y.Score.Value)
	if n := len(y.Goals); n > 0 {
		held, _ := GoalsHeld(y.Goals)
		s += fmt.Sprintf(", goals %d/%d met", held, n)
	}
	return s + "."
}

// MiddayBody is the midday check-in text: tracked time so far, its share of
// the day's target, the top category and focus time.
// "3h05 tracked, 64% of 4h50 target · top: Coding · focus 1h40."
// The share is left out without a target, the top part when no category
// other than Unclassified has time.
func MiddayBody(d Day) string {
	s := Dur(d.Stats.ActiveMS) + " tracked"
	if t := d.Score.Target.MS; t > 0 {
		s += fmt.Sprintf(", %d%% of %s target", (d.Stats.ActiveMS*100+t/2)/t, Dur(t))
	}
	for _, c := range d.Categories {
		if c.Name != Unclassified {
			s += " · top: " + Cut(c.Name, RoutineItemCells)
			break
		}
	}
	return s + " · focus " + Dur(d.Stats.FocusMS) + "."
}
