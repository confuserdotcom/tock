package main

import (
	"fmt"
	"time"

	"tock/internal/report"
)

// cmdTrends prints a month against the previous one and, for the current
// month, what tock learned about your best focus hours; --hours prints the
// weekday x hour grid of mean focus minutes instead.
func cmdTrends(env Env, args []string) error {
	fs := newFlags("trends")
	month := fs.String("month", "", "month to show, YYYY-MM (default this month)")
	hours := fs.Bool("hours", false, "print the weekday x hour grid of mean focus minutes")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("trends: unexpected argument %q", fs.Arg(0))
	}
	day := env.Now()
	if *month != "" {
		d, err := time.ParseInLocation("2006-01", *month, day.Location())
		if err != nil {
			return usagef("trends: --month wants YYYY-MM")
		}
		day = d
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	if *hours {
		p, err := a.Profile(env.Now())
		if err != nil {
			return err
		}
		for _, l := range report.HourGrid(p) {
			fmt.Fprintln(env.Stdout, l)
		}
		return nil
	}
	m, err := a.Month(day)
	if err != nil {
		return err
	}
	for _, l := range report.MonthLines(m) {
		fmt.Fprintln(env.Stdout, l)
	}
	// The profile is about now: a past month prints the month alone.
	now := env.Now()
	if day.Year() != now.Year() || day.Month() != now.Month() {
		return nil
	}
	p, err := a.Profile(now)
	if err != nil {
		return err
	}
	for _, l := range report.ProfileLines(p) {
		fmt.Fprintln(env.Stdout, l)
	}
	return nil
}
