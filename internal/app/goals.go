package app

import (
	"fmt"
	"strings"

	"tock/internal/classify"
	"tock/internal/report"
)

// SetGoal sets goal g in rules.toml: the goal on the same label, direction
// and period (classify.SameGoal) takes g's time, else g is added. g.MS 0
// removes that goal; removing one that does not exist is an error.
func (a *App) SetGoal(g classify.Goal) error {
	if g.Category == "" && g.Project == "" {
		return fmt.Errorf("a goal needs a category, a project or both")
	}
	if g.Category != "" {
		c, err := a.rs.ResolveCategory(g.Category)
		if err != nil {
			return err
		}
		g.Category = c
	}
	if g.Project != "" {
		p, err := a.rs.ResolveProject(g.Project)
		if err != nil {
			return err
		}
		g.Project = p
	}
	limit := int64(24 * 3_600_000)
	if g.Week {
		limit *= 7
	}
	switch {
	case g.MS == 0:
		found := false
		for _, old := range a.rs.Goals {
			found = found || classify.SameGoal(old, g)
		}
		if !found {
			return fmt.Errorf("no goal %s to remove", goalShape(g))
		}
	case g.MS < 60_000 || g.MS > limit:
		return fmt.Errorf("goal %s is not between 1m and %s", classify.FormatDur(g.MS), classify.FormatDur(limit))
	}
	goals := a.rs.Goals
	return a.rewriteRules(func(rules, learned []byte) ([]byte, []byte, error) {
		return []byte(classify.SetGoal(string(rules), goals, g)), learned, nil
	})
}

// goalShape is a goal without its time: "Study ≥ per week".
func goalShape(g classify.Goal) string {
	op, per := "≥", "day"
	if g.AtMost {
		op = "≤"
	}
	if g.Week {
		per = "week"
	}
	return g.Name() + " " + op + " per " + per
}

// modules is each module's time in views, in rules.toml order by
// category: the modules with time or a weekly goal.
func (a *App) modules(vs []classify.SpanView, labels []classify.Label) []report.ModuleRow {
	ms := map[string]int64{}
	for i, v := range vs {
		if p := labels[i].Project; p != "" {
			ms[p] += v.DurMS()
		}
	}
	var out []report.ModuleRow
	for _, c := range a.rs.Categories {
		for _, p := range a.rs.Projects {
			if !strings.EqualFold(classify.NFC(p.Category), classify.NFC(c.Name)) {
				continue
			}
			r := report.ModuleRow{Category: c.Name, Kind: c.Kind, Name: p.Name, MS: ms[p.Name]}
			for _, g := range a.GoalsOf("", p.Name) {
				if g.Week && !g.AtMost {
					r.GoalMS = g.MS
				}
			}
			if r.MS > 0 || r.GoalMS > 0 {
				out = append(out, r)
			}
		}
	}
	return out
}

// GoalsOf returns the goals that count exactly the category (project "")
// or exactly the project (category "").
func (a *App) GoalsOf(category, project string) []classify.Goal {
	var out []classify.Goal
	for _, g := range a.rs.Goals {
		if g.Category == category && g.Project == project {
			out = append(out, g)
		}
	}
	return out
}
