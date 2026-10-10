package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/classify"
	"tock/internal/report"
)

const editUsage = `edit: usage:
  tock edit [--date D]                              list the day's edits
  tock edit add <HH:MM-HH:MM> <Category>[/<Project>] [note…]   time you forgot to track
  tock edit label <HH:MM-HH:MM> <Category>[/<Project>]          label part of a span, or several at once
  tock edit delete <HH:MM-HH:MM>                    remove time (trim a span's start or end)
  tock edit remove <id>                             drop an edit
the end may be "now"; --date picks the day (default today)`

// cmdEdit edits the timeline: entries, labels and cuts over a time range.
// The recorded spans never change; edits apply on top and tock undo
// reverses them.
func cmdEdit(env Env, args []string) error {
	fs := newFlags("edit")
	date := fs.String("date", "", "day, YYYY-MM-DD (default today)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	now := env.Now()
	day := now
	if *date != "" {
		d, err := time.ParseInLocation("2006-01-02", *date, now.Location())
		if err != nil {
			return usagef("edit: --date wants YYYY-MM-DD")
		}
		day = d
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	rest := fs.Args()
	if len(rest) == 0 {
		return listEdits(env, a, day)
	}
	sub, rest := rest[0], rest[1:]
	if sub == "remove" {
		if len(rest) != 1 {
			return usagef(editUsage)
		}
		id, err := strconv.ParseInt(rest[0], 10, 64)
		if err != nil {
			return usagef("edit remove: want an edit id from tock edit")
		}
		if err := a.RemoveEdit(id); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "removed edit %d (tock undo puts it back)\n", id)
		return nil
	}
	kind := map[string]string{"add": classify.EditAdd, "label": classify.EditLabel,
		"delete": classify.EditCut, "trim": classify.EditCut}[sub]
	if kind == "" || len(rest) == 0 || (kind == classify.EditCut && len(rest) != 1) || (kind == classify.EditLabel && len(rest) != 2) ||
		(kind == classify.EditAdd && len(rest) < 2) {
		return usagef(editUsage)
	}
	from, to, err := parseRange(rest[0], day, now)
	if err != nil {
		return usagef("edit: %v", err)
	}
	var want classify.Want
	note := ""
	if kind != classify.EditCut {
		if want, err = parseWant(a.Rules(), rest[1]); err != nil {
			return err
		}
		note = strings.Join(rest[2:], " ")
	}
	if _, err := a.AddEdit(kind, from.UnixMilli(), to.UnixMilli(), want, note); err != nil {
		return err
	}
	r := fmt.Sprintf("%s–%s (%s)", from.Format("15:04"), to.Format("15:04"), report.Dur(to.Sub(from).Milliseconds()))
	switch kind {
	case classify.EditCut:
		fmt.Fprintf(env.Stdout, "deleted %s\n", r)
	case classify.EditAdd:
		fmt.Fprintf(env.Stdout, "added %s → %s\n", r, report.LabelText(want.Category, want.Project))
	default:
		fmt.Fprintf(env.Stdout, "labelled %s → %s\n", r, report.LabelText(want.Category, want.Project))
	}
	fmt.Fprintln(env.Stdout, "tock undo reverses it")
	return nil
}

// parseRange reads "HH:MM-HH:MM" on day; the end may be "now".
func parseRange(s string, day, now time.Time) (time.Time, time.Time, error) {
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return time.Time{}, time.Time{}, errors.New("want a range such as 14:00-15:30")
	}
	clock := func(c string) (time.Time, error) {
		if c == "now" {
			return now.Truncate(time.Minute), nil
		}
		t, err := time.Parse("15:04", c)
		if err != nil || len(c) != 5 {
			return time.Time{}, fmt.Errorf("%q is not HH:MM", c)
		}
		y, m, d := day.Date()
		return time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, day.Location()), nil
	}
	from, err := clock(a)
	if err != nil {
		return from, from, err
	}
	to, err := clock(b)
	if err != nil {
		return from, to, err
	}
	if !to.After(from) {
		return from, to, errors.New("the range ends before it starts")
	}
	return from, to, nil
}

func listEdits(env Env, a *app.App, day time.Time) error {
	es, err := a.Edits(day)
	if err != nil {
		return err
	}
	if len(es) == 0 {
		fmt.Fprintln(env.Stdout, "no edits on "+day.Format("2006-01-02"))
		fmt.Fprintln(env.Stdout, "add one: tock edit add 14:00-15:00 Meeting · tock edit --help")
		return nil
	}
	loc := env.Now().Location()
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	for _, e := range es {
		what := map[string]string{classify.EditAdd: "add", classify.EditCut: "delete", classify.EditLabel: "label"}[e.Kind]
		line := fmt.Sprintf("%4d  %-6s %s–%s  %5s", e.ID, what, clock(e.StartMS), clock(e.EndMS), report.Dur(e.EndMS-e.StartMS))
		if e.Kind != classify.EditCut {
			line += "  " + report.LabelText(e.Category, e.Project)
		}
		if e.Note != "" {
			line += "  " + report.Safe(e.Note)
		}
		fmt.Fprintln(env.Stdout, line)
	}
	fmt.Fprintln(env.Stdout, "drop one: tock edit remove <id>")
	return nil
}

// cmdUndo reverses the newest label, rule, ignore, away or edit change, or
// lists them.
func cmdUndo(env Env, args []string) error {
	fs := newFlags("undo")
	list := fs.Bool("list", false, "list recent changes instead")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("undo takes only --list")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if *list {
		us, err := a.UndoList(20)
		if err != nil {
			return err
		}
		if len(us) == 0 {
			fmt.Fprintln(env.Stdout, "nothing to undo")
		}
		loc := env.Now().Location()
		for _, u := range us {
			fmt.Fprintf(env.Stdout, "%s  %s\n", time.UnixMilli(u.AtMS).In(loc).Format("Mon 15:04"), report.Safe(u.What))
		}
		return nil
	}
	what, err := a.Undo()
	if errors.Is(err, app.ErrNothingToUndo) {
		fmt.Fprintln(env.Stdout, "nothing to undo")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "undid: %s\n", report.Safe(what))
	return nil
}
