package app

import (
	"strconv"
	"testing"
	"time"

	"tock/internal/apptest"
)

// seedHistory writes days days of work before and including the day of Now:
// 09:00-18:00 in 3-minute spans across five apps, so about 180 spans a day.
// It inserts in one transaction (apptest.Seed runs one statement per span,
// which is too slow for half a year).
func seedHistory(b *testing.B, e *apptest.Env, days int) int {
	b.Helper()
	db := e.RawDB(b)
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		b.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO span(start, "end", bundle_id, app, title, url, private) VALUES(?,?,?,?,?,?,0)`)
	if err != nil {
		b.Fatal(err)
	}
	apps := [][3]string{
		{"term", "WezTerm", "tock │ "},
		{"arc", "Arc", "Docs "},
		{"zed", "Zed", "main.go "},
		{"mail", "Mail", "Inbox "},
		{"slack", "Slack", "general "},
	}
	n := 0
	for d := -days + 1; d <= 0; d++ {
		start := apptest.At(b, "09:00", d)
		for i := 0; i < 180; i++ {
			a := apps[(i+d+days)%len(apps)]
			s := start + int64(i)*3*60_000
			var url any
			if a[0] == "arc" {
				url = "docs.example.com/p/" + strconv.Itoa(i%13)
			}
			if _, err := stmt.Exec(s, s+3*60_000, a[0], a[1], a[2]+strconv.Itoa(i%9), url); err != nil {
				b.Fatal(err)
			}
			n++
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	return n
}

func benchOpen(b *testing.B, days int) (*App, *apptest.Env) {
	e := apptest.New(b, apptest.Rules)
	seedHistory(b, e, days)
	e.Clock = time.Date(apptest.Now.Year(), apptest.Now.Month(), apptest.Now.Day(), 18, 0, 0, 0, apptest.Zone)
	a, err := Open(Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config, ShellLog: e.ShellLog}, e.NowFunc())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { a.Close() })
	return a, e
}

// BenchmarkDay builds today, as the TUI and the dashboard do, over a
// database with 180 days of history. It should not grow with the history.
func BenchmarkDay(b *testing.B) {
	for _, days := range []int{7, 180} {
		b.Run(strconv.Itoa(days)+"d", func(b *testing.B) {
			a, e := benchOpen(b, days)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := a.Day(e.Clock); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWeek builds the week view over 180 days of history.
func BenchmarkWeek(b *testing.B) {
	a, e := benchOpen(b, 180)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := a.Week(e.Clock); err != nil {
			b.Fatal(err)
		}
	}
}
