package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tock/internal/report"
)

// cmdReport writes a week as a self-contained HTML page.
func cmdReport(env Env, args []string) error {
	fs := newFlags("report")
	week := fs.String("week", "", "any day of the week, YYYY-MM-DD (default: as tock review)")
	out := fs.String("o", "", "write the page to this file instead of stdout")
	open := fs.Bool("open", false, "open the page in the browser")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usagef("report: unexpected argument %q", fs.Arg(0))
	}
	now := env.Now()
	day := reviewWeek(now)
	if *week != "" {
		d, err := time.ParseInLocation("2006-01-02", *week, now.Location())
		if err != nil {
			return usagef("report: --week wants YYYY-MM-DD")
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
	var b bytes.Buffer
	if err := report.WriteWeekHTML(&b, w, now); err != nil {
		return err
	}
	path := *out
	if path == "" && *open {
		path = filepath.Join(os.TempDir(), "tock-week-"+w.Monday.Format("2006-01-02")+".html")
	}
	if path == "" {
		_, err := env.Stdout.Write(b.Bytes())
		return err
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, "wrote "+path)
	if *open {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		return env.Open("file://" + abs)
	}
	return nil
}
