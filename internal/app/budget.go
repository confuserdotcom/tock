package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/report"
)

// budgetFor is how long Budgets keeps its answer: they move slowly, and
// measuring one reads all the time it counts.
const budgetFor = 5 * time.Minute

type budgetCache struct {
	at   time.Time
	rows []report.BurnRow
}

// Budgets measures every project with a budget, in rules order. The answer
// is kept for budgetFor, and dropped when the rules change.
func (a *App) Budgets() ([]report.BurnRow, error) {
	if c := a.budgets; c != nil && a.now().Sub(c.at) < budgetFor {
		return c.rows, nil
	}
	rows, err := a.measureBudgets()
	if err == nil {
		a.budgets = &budgetCache{at: a.now(), rows: rows}
	}
	return rows, err
}

// measureBudgets sums each budget's project time over the closed days
// (from the past-day cache, so it reads each day once) and today.
func (a *App) measureBudgets() ([]report.BurnRow, error) {
	now := a.now()
	today := a.midnight(now)
	weekFrom := today.AddDate(0, 0, -6)
	type item struct {
		b     report.Budget
		since time.Time
	}
	var items []item
	from := weekFrom
	for _, p := range a.rs.Projects {
		if p.BudgetMS <= 0 {
			continue
		}
		it := item{b: report.Budget{Project: p.Name, BudgetMS: p.BudgetMS, SinceMS: math.MinInt64}}
		if p.BudgetSince != "" {
			if d, err := time.ParseInLocation(time.DateOnly, p.BudgetSince, now.Location()); err == nil {
				it.since, it.b.SinceMS = d, d.UnixMilli()
			}
		}
		if it.since.IsZero() {
			first, ok, err := a.st.FirstMS()
			if err != nil {
				return nil, err
			}
			it.since = today
			if ok {
				it.since = a.midnight(time.UnixMilli(first))
			}
		}
		if it.since.Before(from) {
			from = it.since
		}
		items = append(items, it)
	}
	if len(items) == 0 {
		return nil, nil
	}
	if err := a.fillPast(from, today); err != nil {
		return nil, err
	}
	vs, labels, err := a.labelled(today.UnixMilli(), now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return nil, err
	}
	todayMS := report.ProjectDayMS(vs, labels)
	out := make([]report.BurnRow, len(items))
	for i, it := range items {
		spent, week := todayMS[it.b.Project], todayMS[it.b.Project]
		for d := from; d.Before(today); d = d.AddDate(0, 0, 1) {
			ms := a.past[dateKey(d)].ProjectMS[it.b.Project]
			if !d.Before(it.since) {
				spent += ms
			}
			if !d.Before(weekFrom) {
				week += ms
			}
		}
		out[i] = report.Burn(it.b, spent, week, now.UnixMilli())
	}
	return out, nil
}

// budgetEvery is how often budgetNotices looks.
const budgetEvery = time.Hour

// budgetNotices is a notice for each budget that reached 80% or ran out
// since it last said so. What it said is kept per project in the state
// table, so a budget is announced once per level whichever process looks,
// and again only when the budget or its start changes. A failure is kept
// as the tick's RoutineErr.
func (a *App) budgetNotices(now time.Time) []focus.Event {
	if !a.budgetAt.IsZero() && now.Sub(a.budgetAt) < budgetEvery {
		return nil
	}
	a.budgetAt = now
	rows, err := a.Budgets()
	if err != nil {
		if a.routineErr == nil {
			a.routineErr = err
		}
		return nil
	}
	since := map[string]string{}
	for _, p := range a.rs.Projects {
		since[p.Name] = p.BudgetSince
	}
	var out []focus.Event
	for _, r := range rows {
		level := 0
		switch {
		case r.Over():
			level = 100
		case r.Pct() >= 80:
			level = 80
		default:
			continue
		}
		key := "budget_notice:" + r.Project
		budget := fmt.Sprintf("%d|%s", r.BudgetMS, since[r.Project])
		if old, ok, err := a.st.State(key); err == nil && ok {
			lv, b, _ := strings.Cut(old, "|")
			if n, err := strconv.Atoi(lv); err == nil && b == budget && n >= level {
				continue
			}
		}
		if err := a.st.SetState(key, fmt.Sprintf("%d|%s", level, budget), now.UnixMilli()); err != nil {
			if a.routineErr == nil {
				a.routineErr = err
			}
			continue
		}
		name, b := report.Safe(r.Project), classify.FormatDur(r.BudgetMS)
		ev := focus.Event{Kind: focus.Notify, Topic: focus.TopicBudget,
			Project: r.Project, BudgetMS: r.BudgetMS, SpentMS: r.SpentMS, RunsOutMS: r.RunsOutMS}
		if level == 100 {
			ev.Title = fmt.Sprintf("%s is over its %s budget", name, b)
			ev.Body = fmt.Sprintf("%s spent, over by %s.", report.Dur(r.SpentMS), report.Dur(r.SpentMS-r.BudgetMS))
		} else {
			ev.Title = fmt.Sprintf("%s: %d%% of its %s budget", name, r.Pct(), b)
			ev.Body = fmt.Sprintf("%s spent, %s left.", report.Dur(r.SpentMS), report.Dur(r.BudgetMS-r.SpentMS))
			if r.RunsOutMS > 0 {
				ev.Body += " At this week's pace it runs out " + time.UnixMilli(r.RunsOutMS).In(now.Location()).Format("Mon 2 Jan") + "."
			}
		}
		out = append(out, ev)
	}
	return out
}
