package main

import (
	"fmt"
	"strings"
	"time"

	"tock/internal/report"
)

const (
	keysPerRow  = 5
	topProjects = 3
	barCells    = 12
	// The category column fits the widest name shown, within these cells.
	nameMin = 14
	nameMax = 30
)

func cmdToday(env Env, args []string) error {
	fs := newFlags("today")
	date := fs.String("date", "", "day to show, YYYY-MM-DD (default today)")
	withKeys := fs.Bool("keys", false, "list the top keys of each category, numbered for tock tag")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("today: unexpected argument %q", fs.Arg(0))
	}

	now := env.Now()
	day := now
	if *date != "" {
		d, err := time.ParseInLocation("2006-01-02", *date, now.Location())
		if err != nil {
			return usagef("today: --date wants YYYY-MM-DD")
		}
		day = d
	}

	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	d, err := a.Day(day)
	if err != nil {
		return err
	}

	title := "Today"
	if *date != "" {
		title = d.Date.Format("2006-01-02")
	}
	fmt.Fprintf(env.Stdout, "%s  %s tracked · %s idle%s\n", title, report.Dur(d.TrackedMS), report.Dur(d.IdleMS), cutNote(d.Cut))
	if len(d.Categories) == 0 {
		fmt.Fprintln(env.Stdout, "(no activity)")
		return nil
	}
	fmt.Fprintln(env.Stdout, report.ScoreLine(d))
	if len(d.Goals) > 0 {
		fmt.Fprintln(env.Stdout, "goals  "+report.GoalsLine(d.Goals))
	}
	if len(d.Commits) > 0 {
		fmt.Fprintln(env.Stdout, "commits  "+report.CommitsLine(d.Commits))
	}

	var maxMS int64
	names := make([]string, 0, len(d.Categories))
	for _, r := range d.Categories {
		maxMS = max(maxMS, r.MS)
		names = append(names, r.Name)
	}
	nameW := report.NameWidth(names, nameMin, nameMax)
	var listed []string
	for _, r := range d.Categories {
		extra := projectSummary(r.Projects)
		if r.Name == report.Unclassified && len(r.Items) > 0 {
			extra = fmt.Sprintf("%d keys → tock inbox", len(r.Items))
		}
		if r.CallMS > 0 {
			extra = strings.TrimLeft(extra+" · "+report.Dur(r.CallMS)+" on calls", " ·")
		}
		line := fmt.Sprintf("%s %5s  %s  %s", report.Fit(r.Name, nameW), report.Dur(r.MS),
			report.Pad(report.Bar(r.MS, maxMS, barCells), barCells), extra)
		fmt.Fprintln(env.Stdout, strings.TrimRight(line, " "))
		if *withKeys {
			for _, it := range r.Items[:min(len(r.Items), keysPerRow)] {
				listed = append(listed, it.Key)
				fmt.Fprintf(env.Stdout, "    %3d  %5s  %s\n", len(listed), report.Dur(it.TotalMS), report.Safe(it.Key))
			}
		}
	}
	if *withKeys {
		return saveListing(env, listed)
	}
	return nil
}

// projectSummary renders the top projects as "tock 1h40 · ATLAS 34m".
func projectSummary(p []report.NameMS) string {
	parts := make([]string, 0, topProjects)
	for _, n := range p[:min(len(p), topProjects)] {
		parts = append(parts, n.Name+" "+report.Dur(n.MS))
	}
	return strings.Join(parts, " · ")
}

// cutNote says what recorded time did not count, when a minute or more.
func cutNote(c report.Uncounted) string {
	var s string
	if c.StaleMS >= 60_000 {
		s += " · " + report.Dur(c.StaleMS) + " cut (no input)"
	}
	if c.OtherMS >= 60_000 {
		s += " · " + report.Dur(c.OtherMS) + " to other machines"
	}
	return s
}
