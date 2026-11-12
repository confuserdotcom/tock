package main

import (
	"fmt"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
)

const budgetUsage = "budget: usage: tock budget   (set one with tock project set <name> --budget 40h [--budget-since YYYY-MM-DD])"

// cmdBudget shows how much of each project's budget is spent and when the
// rest runs out at this week's pace.
func cmdBudget(env Env, args []string) error {
	if len(args) > 0 {
		return usagef(budgetUsage)
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	rows, err := a.Budgets()
	if err != nil {
		return err
	}
	for _, l := range budgetLines(rows, env.Now()) {
		fmt.Fprintln(env.Stdout, l)
	}
	return nil
}

func budgetLines(rows []report.BurnRow, now time.Time) []string {
	if len(rows) == 0 {
		return []string{"no project has a budget: tock project set <name> --budget 40h"}
	}
	w := 0
	for _, r := range rows {
		w = max(w, len([]rune(report.Safe(r.Project))))
	}
	const barW = 20
	var out []string
	for _, r := range rows {
		filled := min(barW, int(int64(barW)*r.SpentMS/max(r.BudgetMS, 1)))
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barW-filled)
		line := fmt.Sprintf("%-*s  %6s / %-6s %4d%%  %s", w, report.Safe(r.Project), report.Dur(r.SpentMS), classify.FormatDur(r.BudgetMS), r.Pct(), bar)
		switch {
		case r.Over():
			line += "  over by " + report.Dur(r.SpentMS-r.BudgetMS)
		case r.RunsOutMS > 0:
			end := time.UnixMilli(r.RunsOutMS).In(now.Location())
			line += fmt.Sprintf("  %s/day · runs out %s", report.Dur(r.PerDayMS), end.Format("Mon 2 Jan"))
		default:
			line += "  no time this week"
		}
		out = append(out, line)
	}
	return out
}
