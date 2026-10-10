package main

import (
	"bufio"
	"fmt"
	"time"

	"tock/internal/report"
)

// reviewWeek is the week tock review shows by default: this week on a
// weekend, else last week.
func reviewWeek(now time.Time) time.Time {
	if wd := now.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return now
	}
	return now.AddDate(0, 0, -7)
}

// cmdReview prints the weekly review and asks for next week's plan.
func cmdReview(env Env, args []string) error {
	fs := newFlags("review")
	week := fs.String("week", "", "any day of the week to review, YYYY-MM-DD")
	plan := fs.String("plan", "", "next week's focus, without asking")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("review: unexpected argument %q", fs.Arg(0))
	}
	now := env.Now()
	day := reviewWeek(now)
	if *week != "" {
		d, err := time.ParseInLocation("2006-01-02", *week, now.Location())
		if err != nil {
			return usagef("review: --week wants YYYY-MM-DD")
		}
		day = d
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	w, err := a.Week(day)
	if err != nil {
		return err
	}
	for _, l := range report.ReviewLines(w) {
		fmt.Fprintln(env.Stdout, l)
	}

	text := *plan
	if text == "" {
		if !env.Interactive {
			return nil
		}
		fmt.Fprint(env.Stdout, "\nnext week's focus (enter to skip): ")
		line, _ := bufio.NewReader(env.Stdin).ReadString('\n')
		text = line
	}
	text = report.CleanPlan(text)
	if text == "" {
		return nil
	}
	next := w.Monday.AddDate(0, 0, 7)
	if err := a.SetPlan(next, text); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "saved plan for the week of %s\n", next.Format("Mon 2 Jan"))
	return nil
}
