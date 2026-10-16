package main

import (
	"fmt"
	"strings"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

const goalUsage = `usage: tock goal                                       list the goals and this week's progress
       tock goal set <label> <time>[/day|/week] [--max]  e.g. tock goal set ana1 6h/week
       tock goal rm <label> [day|week] [--max]
<label> is a category, a module or project (or one of its other names), or Category/Project;
--max makes it a limit: at most <time>`

// cmdGoal lists, sets and removes the [[goal]] tables of rules.toml.
func cmdGoal(env Env, args []string) error {
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if len(args) == 0 {
		return goalList(env, a)
	}
	sub, rest := args[0], args[1:]
	fs := newFlags("goal " + sub)
	most := fs.Bool("max", false, "a limit: at most the time")
	if err := parseFlags(fs, moveFlagsFirst(rest)); err != nil {
		return err
	}
	var g classify.Goal
	switch sub {
	case "set":
		if fs.NArg() != 2 {
			return usagef(goalUsage)
		}
		dur, per, _ := strings.Cut(fs.Arg(1), "/")
		if g.MS, err = classify.ParseDur(dur); err != nil {
			return usagef("goal: %v", err)
		}
		if g.Week, err = goalPer(per); err != nil {
			return err
		}
	case "rm", "remove":
		if fs.NArg() < 1 || fs.NArg() > 2 {
			return usagef(goalUsage)
		}
		if g.Week, err = goalPer(fs.Arg(1)); err != nil {
			return err
		}
	default:
		return usagef(goalUsage)
	}
	g.AtMost = *most
	if g.Category, g.Project, err = goalLabel(a.Rules(), fs.Arg(0)); err != nil {
		return err
	}
	if err := a.SetGoal(g); err != nil {
		return err
	}
	if g.MS == 0 {
		fmt.Fprintf(env.Stdout, "removed the goal on %s\n", report.Safe(g.Name()))
	} else {
		fmt.Fprintf(env.Stdout, "goal %s\n", report.Safe(g.Text()))
	}
	return nil
}

func goalPer(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "", "day", "d":
		return false, nil
	case "week", "wk", "w":
		return true, nil
	}
	return false, usagef("goal: per %q, want day or week", s)
}

// goalLabel reads a goal's label: Category/Project; else a category named
// exactly, a module by name or other name, a category or a project.
func goalLabel(rs *classify.Ruleset, arg string) (string, string, error) {
	if strings.Contains(arg, "/") {
		w, err := parseWant(rs, arg)
		return w.Category, w.Project, err
	}
	c, cerr := rs.ResolveCategory(arg)
	if cerr == nil && strings.EqualFold(classify.NFC(c), classify.NFC(strings.TrimSpace(arg))) {
		return c, "", nil
	}
	if p := rs.ModuleOf(arg); p != "" {
		return "", p, nil
	}
	if cerr == nil {
		return c, "", nil
	}
	if p, err := rs.ResolveProject(arg); err == nil {
		return "", p, nil
	}
	return "", "", fmt.Errorf("no category or project %q", arg)
}

func goalList(env Env, a *app.App) error {
	w, err := a.Week(env.Now())
	if err != nil {
		return err
	}
	if len(w.Goals) == 0 {
		fmt.Fprintln(env.Stdout, "no goals · tock goal set <label> <time>[/day|/week]")
		return nil
	}
	width := 0
	for _, g := range w.Goals {
		width = max(width, len([]rune(g.Goal.Text())))
	}
	for _, g := range w.Goals {
		fmt.Fprintf(env.Stdout, "%-*s  %s\n", width, report.Safe(g.Goal.Text()), report.WeekGoalText(g))
	}
	return nil
}
