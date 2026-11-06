package main

import "tock/internal/report"

// cmdExport writes labelled spans as csv or json to stdout.
func cmdExport(env Env, args []string) error {
	fs := newFlags("export")
	fromS := fs.String("from", "", "first day, YYYY-MM-DD")
	toS := fs.String("to", "", "last day, YYYY-MM-DD")
	format := fs.String("format", "csv", "csv or json")
	by := fs.String("by", "", `"project": one row per project with hours, rate and amount`)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("export takes only flags")
	}
	o, err := report.ParseExport(*fromS, *toS, *format, *by, env.Now())
	if err != nil {
		return usagef("export: %v", err)
	}
	a, err := openApp(env)
	if err != nil {
		return err
	}
	defer a.Close()
	rows, err := a.Export(o.From, o.End())
	if err != nil {
		return err
	}
	return report.WriteExport(env.Stdout, rows, o)
}
