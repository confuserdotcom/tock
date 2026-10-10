package main

import (
	"fmt"
	"slices"
	"time"

	"tock/internal/report"
)

const topUsage = "top: usage: tock top [--days N | --week | --month] [--by category|app|site|project] [--limit N]"

// cmdTop ranks where the time went: categories, apps, sites and projects.
func cmdTop(env Env, args []string) error {
	fs := newFlags("top")
	days := fs.Int("days", 7, "the last N days, today included")
	week := fs.Bool("week", false, "this week, from Monday")
	month := fs.Bool("month", false, "this calendar month")
	by := fs.String("by", "", "one section only: category, app, site or project")
	limit := fs.Int("limit", 10, "rows per section (0 = all)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 || *days < 1 || *days > 366 || *limit < 0 || (*week && *month) {
		return usagef(topUsage)
	}
	sections := []string{report.TopCategory, report.TopApp, report.TopSite}
	if *by != "" {
		if !slices.Contains(report.TopSections, *by) {
			return usagef(topUsage)
		}
		sections = []string{*by}
	}
	now := env.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	to := today.AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -*days)
	switch {
	case *week:
		from = today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	case *month:
		from = today.AddDate(0, 0, 1-today.Day())
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	t, err := a.Top(from, to)
	if err != nil {
		return err
	}
	for _, l := range report.TopLines(t, sections, *limit) {
		fmt.Fprintln(env.Stdout, l)
	}
	return nil
}
