package report

import (
	"tock/internal/classify"
)

// Budget is a project's budget = …: BudgetMS of time from SinceMS on.
type Budget struct {
	Project           string
	BudgetMS, SinceMS int64
}

// BurnRow is how much of a budget is spent. PerDayMS is the project's
// average a day over the last 7 days; RunsOutMS is when the rest runs out
// at that pace, 0 when it is spent or there is no pace.
type BurnRow struct {
	Budget
	SpentMS, PerDayMS, RunsOutMS int64
}

// Pct is the share spent, in whole percent.
func (b BurnRow) Pct() int { return int(b.SpentMS * 100 / max(b.BudgetMS, 1)) }

// Over reports a budget spent past its end.
func (b BurnRow) Over() bool { return b.SpentMS > b.BudgetMS }

const dayMS = 24 * 3600_000

// Burns measures each budget at nowMS from views labelled up to then, in
// the order given.
func Burns(budgets []Budget, views []classify.SpanView, labels []classify.Label, nowMS int64) []BurnRow {
	out := make([]BurnRow, len(budgets))
	for i, b := range budgets {
		r := BurnRow{Budget: b}
		r.SpentMS = projectMS(views, labels, b.Project, b.SinceMS, nowMS)
		r.PerDayMS = projectMS(views, labels, b.Project, nowMS-7*dayMS, nowMS) / 7
		if left := b.BudgetMS - r.SpentMS; left > 0 && r.PerDayMS > 0 {
			r.RunsOutMS = nowMS + left/r.PerDayMS*dayMS + left%r.PerDayMS*dayMS/r.PerDayMS
		}
		out[i] = r
	}
	return out
}
