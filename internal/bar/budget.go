package bar

import (
	"strconv"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
	"tock/internal/store"
)

// budgetCells is the width of the budget gauge in strip cells.
const budgetCells = 24

// BudgetIsland is the moment a project's budget reaches 80% or runs out. A
// notice queued by an older trackd has no numbers: its text stands in.
func BudgetIsland(n store.Notice, d app.NoticeData, loc *time.Location) Island {
	name := d.Project
	if name == "" {
		name = n.Title
	}
	f := Face{Dot: DotBreak, Left: report.Cut(report.Safe(name), goalCells)}
	p := &Panel{Lead: n.Title, Sub: n.Body}
	if d.BudgetMS > 0 {
		pct := d.SpentMS * 100 / d.BudgetMS
		f.Right = strconv.FormatInt(pct, 10) + "%"
		over := d.SpentMS > d.BudgetMS
		if over {
			f.Dot = "distract"
		}
		p.Sub = ""
		p.Cells = []Cell{{K: "Spent", V: report.Dur(d.SpentMS)}, {K: "Budget", V: classify.FormatDur(d.BudgetMS)}}
		if over {
			p.Cells = append(p.Cells, Cell{K: "Over", V: report.Dur(d.SpentMS - d.BudgetMS), Tone: "bad"})
		} else {
			p.Cells = append(p.Cells, Cell{K: "Left", V: report.Dur(d.BudgetMS - d.SpentMS)})
			if d.RunsOutMS > 0 {
				p.Cells = append(p.Cells, Cell{K: "Runs out", V: time.UnixMilli(d.RunsOutMS).In(loc).Format("Mon 2 Jan")})
			}
		}
		p.Strip, p.Axis = budgetGauge(d.BudgetMS, d.SpentMS)
	}
	p.Face = f
	p.Buttons = []NButton{{ID: IDDashboard, Title: "Open Today", Primary: true}, closeButton}
	return Island{State: IslandBudget, Open: true, Notice: n.ID, AutoClose: meetAutoClose, Face: f, Panel: p}
}

// budgetGauge draws spent against budget as strip cells: spent within the
// budget amber, the rest dark. Over budget the gauge spans what was spent
// and the cells past the budget line turn red.
func budgetGauge(budgetMS, spentMS int64) ([]string, []string) {
	scale := max(budgetMS, spentMS)
	cells := make([]string, budgetCells)
	for i := range cells {
		mid := (int64(i)*2 + 1) * scale / (2 * budgetCells)
		switch {
		case mid < min(spentMS, budgetMS):
			cells[i] = DotBreak
		case mid < spentMS:
			cells[i] = "distract"
		default:
			cells[i] = "none"
		}
	}
	return cells, []string{"0", classify.FormatDur(scale)}
}
