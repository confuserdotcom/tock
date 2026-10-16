package main

import (
	"fmt"
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/report"
)

const awayUsage = "away: usage: tock away [--date YYYY-MM-DD] [<HH:MM> <Category>[/<Project>] | <HH:MM> same | --skip <HH:MM>]"

// cmdAway lists the day's unlabelled time away, labels one absence or
// leaves it idle.
func cmdAway(env Env, args []string) error {
	fs := newFlags("away")
	date := fs.String("date", "", "day, YYYY-MM-DD (default today)")
	skip := fs.Bool("skip", false, "leave the absence idle and stop asking")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	want := 0
	switch {
	case *skip:
		want = 1
	case fs.NArg() > 0:
		want = 2
	}
	if fs.NArg() != want {
		return usagef(awayUsage)
	}
	now := env.Now()
	day := now
	if *date != "" {
		d, err := time.ParseInLocation("2006-01-02", *date, now.Location())
		if err != nil {
			return usagef("away: --date wants YYYY-MM-DD")
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
	loc := now.Location()
	clock := func(ms int64) string { return time.UnixMilli(ms).In(loc).Format("15:04") }
	span := func(iv classify.Interval) string {
		return fmt.Sprintf("%s–%s (%s)", clock(iv.From), clock(iv.To), report.Dur(iv.To-iv.From))
	}
	if want == 0 {
		if len(d.Away) == 0 {
			fmt.Fprintln(env.Stdout, "no time away to label")
			return nil
		}
		same := false
		for i, iv := range d.Away {
			after := ""
			if b := d.AwayBefore[i]; b.Category != "" {
				after, same = "  after "+report.LabelText(b.Category, b.Project), true
			}
			fmt.Fprintf(env.Stdout, "%s–%s  %5s%s\n", clock(iv.From), clock(iv.To), report.Dur(iv.To-iv.From), after)
		}
		first := clock(d.Away[0].From)
		fmt.Fprintf(env.Stdout, "label: tock away %s <Category>[/<Project>] · leave idle: tock away --skip %s\n", first, first)
		if same {
			fmt.Fprintln(env.Stdout, "still on what came before: tock away <HH:MM> same")
		}
		return nil
	}

	var iv *classify.Interval
	var before classify.Want
	var starts []string
	for i := range d.Away {
		starts = append(starts, clock(d.Away[i].From))
		if clock(d.Away[i].From) == fs.Arg(0) {
			iv, before = &d.Away[i], d.AwayBefore[i]
		}
	}
	if iv == nil {
		if len(starts) == 0 {
			return fmt.Errorf("no time away to label on %s", d.Date.Format("2006-01-02"))
		}
		return fmt.Errorf("no time away starts at %s (try %s)", fs.Arg(0), strings.Join(starts, ", "))
	}
	if *skip {
		if err := a.Answer(*iv, classify.Want{}); err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "left %s idle\n", span(*iv))
		return nil
	}
	var w classify.Want
	switch {
	case fs.Arg(1) == "same" && !a.Rules().HasCategory("same"):
		if before.Category == "" {
			return fmt.Errorf("nothing labelled right before %s: name the label", span(*iv))
		}
		w = before
	default:
		if w, err = parseWant(a.Rules(), fs.Arg(1)); err != nil {
			return err
		}
	}
	if err := a.Answer(*iv, w); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "labelled %s → %s\n", span(*iv), report.LabelText(w.Category, w.Project))
	return nil
}
