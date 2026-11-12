package report

import (
	"tock/internal/classify"
	"tock/internal/span"
)

// Budget is a project's budget = …: BudgetMS of time from SinceMS on.
type Budget struct {
	Project           string
	BudgetMS, SinceMS int64
}

// BurnRow is how much of a budget is spent. PerDayMS is the project's
// average a day over the last 7 days; RunsOutMS is when the rest runs out
// at that pace, 0 when it is spent or the pace is under MinPaceMS.
type BurnRow struct {
	Budget
	SpentMS, PerDayMS, RunsOutMS int64
}

// Pct is the share spent, in whole percent.
func (b BurnRow) Pct() int { return int(b.SpentMS * 100 / max(b.BudgetMS, 1)) }

// Over reports a budget spent past its end.
func (b BurnRow) Over() bool { return b.SpentMS > b.BudgetMS }

const dayMS = 24 * 3600_000

// MinPaceMS is the least time a day that forecasts a run-out day; less is
// a project left alone, whose forecast would be years away.
const MinPaceMS = 60_000

// Burn measures b at nowMS from the time spent since b.SinceMS and the
// project's time over the last 7 days.
func Burn(b Budget, spentMS, weekMS, nowMS int64) BurnRow {
	r := BurnRow{Budget: b, SpentMS: spentMS, PerDayMS: weekMS / 7}
	if left := b.BudgetMS - r.SpentMS; left > 0 && r.PerDayMS >= MinPaceMS {
		r.RunsOutMS = nowMS + left/r.PerDayMS*dayMS + left%r.PerDayMS*dayMS/r.PerDayMS
	}
	return r
}

// ProjectDayMS is the non-idle time of each project in views; nil when
// there is none.
func ProjectDayMS(views []classify.SpanView, labels []classify.Label) map[string]int64 {
	var out map[string]int64
	for i, v := range views {
		if v.BundleID == span.IdleBundleID || labels[i].Project == "" || v.End <= v.Start {
			continue
		}
		if out == nil {
			out = map[string]int64{}
		}
		out[labels[i].Project] += v.End - v.Start
	}
	return out
}
