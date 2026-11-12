package app

import (
	"math"
	"time"

	"tock/internal/report"
)

// Budgets measures every project with a budget, in rules order.
func (a *App) Budgets() ([]report.BurnRow, error) {
	now := a.nowMS()
	var bs []report.Budget
	from := now - 7*24*3600_000
	for _, p := range a.rs.Projects {
		if p.BudgetMS <= 0 {
			continue
		}
		b := report.Budget{Project: p.Name, BudgetMS: p.BudgetMS, SinceMS: math.MinInt64}
		if p.BudgetSince != "" {
			d, err := time.ParseInLocation(time.DateOnly, p.BudgetSince, a.now().Location())
			if err == nil {
				b.SinceMS = d.UnixMilli()
			}
		}
		from = min(from, b.SinceMS)
		bs = append(bs, b)
	}
	if len(bs) == 0 {
		return nil, nil
	}
	vs, labels, err := a.labelled(from, now, now)
	if err != nil {
		return nil, err
	}
	return report.Burns(bs, vs, labels, now), nil
}
