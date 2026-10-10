package main

import (
	"time"

	"tock/internal/report"
)

// cmdExport writes labelled spans as csv or json to stdout.
func cmdExport(env Env, args []string) error {
	fs := newFlags("export")
	fromS := fs.String("from", "", "first day, YYYY-MM-DD")
	toS := fs.String("to", "", "last day, YYYY-MM-DD")
	format := fs.String("format", "csv", "csv or json")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("export takes only flags")
	}
	if *format != "csv" && *format != "json" {
		return usagef("export: format %q, want csv or json", *format)
	}
	now := env.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	day := func(s string, def time.Time) (time.Time, error) {
		if s == "" {
			return def, nil
		}
		d, err := time.ParseInLocation("2006-01-02", s, now.Location())
		if err != nil {
			return time.Time{}, usagef("export: date %q, want YYYY-MM-DD", s)
		}
		return d, nil
	}
	from, err := day(*fromS, today.AddDate(0, 0, -6))
	if err != nil {
		return err
	}
	to, err := day(*toS, today)
	if err != nil {
		return err
	}
	if from.After(to) {
		return usagef("export: --from is after --to")
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	rows, err := a.Export(from, to.AddDate(0, 0, 1))
	if err != nil {
		return err
	}
	if *format == "json" {
		return report.WriteJSON(env.Stdout, rows)
	}
	return report.WriteCSV(env.Stdout, rows)
}
