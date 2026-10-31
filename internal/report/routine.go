package report

import (
	"fmt"

	"tock/internal/plan"
)

// The start-of-day notice lists at most RoutineItems timed plan items, each
// cut to RoutineItemCells cells.
const (
	RoutineItems     = 3
	RoutineItemCells = 24
)

// StartBody is the start-of-day text: yesterday's result.
// "Yesterday: 6h12 tracked, score 74, goals 2/3 met."
func StartBody(y Day, today []plan.Row) string {
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
