package main

import (
	"fmt"
	"time"

	"tock/internal/report"
)

// cmdTrends prints a month against the previous one.
func cmdTrends(env Env, args []string) error {
	fs := newFlags("trends")
	month := fs.String("month", "", "month to show, YYYY-MM (default this month)")
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
	m, err := a.Month(day)
	if err != nil {
		return err
	}
	for _, l := range report.MonthLines(m) {
		fmt.Fprintln(env.Stdout, l)
	}
	return nil
}
