// Package apptest seeds temporary tock data directories for tests of the
// app and tui packages. Everything it writes is synthetic.
package apptest

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/store"
)

// Zone is fixed so day boundaries are deterministic.
var Zone = time.FixedZone("T", 2*3600)

// NoRoutines is the [routine] config lines that turn the daily routine
// notices off, for tests that assert exact notice lists.
const NoRoutines = "[routine]\nstart = false\nmidday = \"off\"\noverwork = false\n"

// NoPlanNotices is the [plan] config lines that turn the plan and calendar
// notices off, for tests that assert exact notice lists. A test that already
// writes a [plan] table adds `notify = false` to it instead, since TOML
// refuses a second [plan] table.
const NoPlanNotices = "[plan]\nnotify = false\n"

// NoRecommend is the [recommend] config lines that turn recommendations off,
// for tests that assert exact notice lists. A test that already writes a
// [recommend] table adds `enabled = false` to it instead, since TOML refuses
// a second [recommend] table.
const NoRecommend = "[recommend]\nenabled = false\n"

// NoNudgeLimit is the [focus] lines that turn the nudge budget and gap off,
// for tests of a notice's own timing; add them to the test's [focus] table.
const NoNudgeLimit = "notice_budget = 0\nnotice_gap = 0\n"

// AllHours is an [hours] table with every weekday 08:00-18:00.
const AllHours = "[hours]\ndays = [\"mon\", \"tue\", \"wed\", \"thu\", \"fri\", \"sat\", \"sun\"]\nstart = \"08:00\"\nend = \"18:00\"\n"

// Now is the default test clock: Saturday 2026-10-10 15:00 local.
var Now = time.Date(2026, 10, 10, 15, 0, 0, 0, Zone)

// Env is one temporary data directory with its file paths and a settable
// clock.
type Env struct {
	DB, Rules, Learned, Config string
	ShellLog                   string
	Clock                      time.Time
}

// New returns an Env in t.TempDir with the clock at Now. rules is written
// to rules.toml unless empty.
func New(t testing.TB, rules string) *Env {
	t.Helper()
	root := t.TempDir()
	e := &Env{
		DB:       filepath.Join(root, "data", "tock.db"),
		Rules:    filepath.Join(root, "config", "rules.toml"),
		Learned:  filepath.Join(root, "config", "learned.toml"),
		Config:   filepath.Join(root, "config", "config.toml"),
		ShellLog: filepath.Join(root, "state", "shell.log"),
		Clock:    Now,
	}
	if rules != "" {
		e.WriteRules(t, rules)
	}
	return e
}

// NowFunc returns a clock that reads e.Clock, so tests can move time.
func (e *Env) NowFunc() func() time.Time { return func() time.Time { return e.Clock } }

// WriteRules replaces rules.toml.
func (e *Env) WriteRules(t testing.TB, rules string) {
	t.Helper()
	write(t, e.Rules, rules)
}

// WriteConfig replaces config.toml.
func (e *Env) WriteConfig(t testing.TB, body string) {
	t.Helper()
	write(t, e.Config, body)
}

// WriteShellLog replaces the shell log with one "<ms>\t<dir>" line per
// event: at is "HH:MM" on the day of Now.
func (e *Env) WriteShellLog(t testing.TB, events ...ShellEvent) {
	t.Helper()
	var b strings.Builder
	for _, ev := range events {
		fmt.Fprintf(&b, "%d\t%s\n", At(t, ev.At, 0), ev.Dir)
	}
	write(t, e.ShellLog, b.String())
}

// ShellEvent is one seeded shell hook line.
type ShellEvent struct{ At, Dir string }

func write(t testing.TB, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Span is one seeded span. Start and End are "HH:MM" on the day of Now
// (End "" = open); Day shifts both by whole days.
type Span struct {
	ID         int64
	Start, End string
	Bundle     string
	App        string
	Title, URL string // "" = NULL
	Private    bool
	Day        int
}

// At turns "HH:MM" into unix ms on the day of Now plus day days.
func At(t testing.TB, hhmm string, day int) int64 {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02 15:04", Now.Format("2006-01-02")+" "+hhmm, Zone)
	if err != nil {
		t.Fatal(err)
	}
	return tm.AddDate(0, 0, day).UnixMilli()
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (e *Env) raw(t testing.TB) *sql.DB {
	t.Helper()
	st, err := store.Open(e.DB) // creates and migrates
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", "file:"+e.DB)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// RawDB opens tock.db (created if needed) for direct SQL.
func (e *Env) RawDB(t testing.TB) *sql.DB { return e.raw(t) }

// Seed creates tock.db if needed and inserts spans.
func (e *Env) Seed(t testing.TB, spans ...Span) {
	t.Helper()
	db := e.raw(t)
	defer db.Close()
	for _, s := range spans {
		var end any
		if s.End != "" {
			end = At(t, s.End, s.Day)
		}
		priv := 0
		if s.Private {
			priv = 1
		}
		if _, err := db.Exec(`INSERT INTO span(id, start, "end", bundle_id, app, title, url, private) VALUES(?,?,?,?,?,?,?,?)`,
			s.ID, At(t, s.Start, s.Day), end, s.Bundle, s.App, nullable(s.Title), nullable(s.URL), priv); err != nil {
			t.Fatal(err)
		}
	}
}

// State sets one daemon_state row, as trackd would.
func (e *Env) State(t testing.TB, key, value string) {
	t.Helper()
	db := e.raw(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO daemon_state(key, value, updated) VALUES(?,?,0)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
		t.Fatal(err)
	}
}

// Call records a call from "HH:MM" to "HH:MM" on the day of Now, as trackd
// would.
func (e *Env) Call(t testing.TB, from, to string) {
	t.Helper()
	db := e.raw(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO call(start_ms, end_ms) VALUES(?, ?)`, At(t, from, 0), At(t, to, 0)); err != nil {
		t.Fatal(err)
	}
}

// Event records a calendar event from "HH:MM" to "HH:MM" on the day of Now,
// as trackd would.
func (e *Env) Event(t testing.TB, from, to, title string) {
	t.Helper()
	db := e.raw(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO event(start_ms, end_ms, title, calendar) VALUES(?, ?, ?, 'Work')`,
		At(t, from, 0), At(t, to, 0), title); err != nil {
		t.Fatal(err)
	}
}

// EventLink is like Event, with the meeting link trackd would have found, for
// an event the owner accepted or organizes.
func (e *Env) EventLink(t testing.TB, from, to, title, link string) {
	t.Helper()
	e.eventLink(t, from, to, title, link, 1)
}

// EventLinkUnanswered is EventLink for an invite the owner has not accepted.
func (e *Env) EventLinkUnanswered(t testing.TB, from, to, title, link string) {
	t.Helper()
	e.eventLink(t, from, to, title, link, 0)
}

func (e *Env) eventLink(t testing.TB, from, to, title, link string, mine int) {
	t.Helper()
	db := e.raw(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO event(start_ms, end_ms, title, calendar, join_url, mine) VALUES(?, ?, ?, 'Work', ?, ?)`,
		At(t, from, 0), At(t, to, 0), title, link, mine); err != nil {
		t.Fatal(err)
	}
}

// Plan saves the plan of the week starting on monday (YYYY-MM-DD).
func (e *Env) Plan(t testing.TB, monday, text string) {
	t.Helper()
	db := e.raw(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO plan(week, text, at_ms) VALUES(?, ?, 1)`, monday, text); err != nil {
		t.Fatal(err)
	}
}

// Summary saves a Claude summary for day (YYYY-MM-DD): the day's text at
// start 0, else the work block's that starts at start.
func (e *Env) Summary(t testing.TB, day string, start int64, text string) {
	t.Helper()
	db := e.raw(t)
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO summary(day, start_ms, text, model, created_ms) VALUES(?, ?, ?, 'test', 1)`,
		day, start, text); err != nil {
		t.Fatal(err)
	}
}

// Heartbeat records a trackd heartbeat at ms.
func (e *Env) Heartbeat(t testing.TB, ms int64) {
	t.Helper()
	e.State(t, store.KeyHeartbeat, strconv.FormatInt(ms, 10))
}

// Run is one trackd_run row for Runs: "HH:MM" times on Day (days from
// Now); Stop "" leaves it open with its last tick at Last.
type Run struct {
	Start, Last, Stop string
	Day               int
	Reason, State     string
	Boot              int64
}

// Runs inserts trackd_run rows, as trackd would.
func (e *Env) Runs(t testing.TB, runs ...Run) {
	t.Helper()
	db := e.raw(t)
	defer db.Close()
	for _, r := range runs {
		var stop, reason any
		if r.Stop != "" {
			stop, reason = At(t, r.Stop, r.Day), r.Reason
		}
		last := r.Last
		if last == "" {
			last = r.Stop
		}
		state := r.State
		if state == "" {
			state = store.RunRecording
		}
		if _, err := db.Exec(`INSERT INTO trackd_run(start_ms, last_ms, stop_ms, reason, state, boot_ms) VALUES(?,?,?,?,?,?)`,
			At(t, r.Start, r.Day), At(t, last, r.Day), stop, reason, state, r.Boot); err != nil {
			t.Fatal(err)
		}
	}
}

// Rules is a small rule set covering the seeded day.
const Rules = `
[[category]]
name = "Code"
kind = "deep"

[[category]]
name = "Study"
kind = "deep"

[[category]]
name = "Communication"
kind = "shallow"

[[category]]
name = "Entertainment"
kind = "distract"

[[project]]
name = "tock"

[[project]]
name = "ATLAS"

[[project]]
name = "Old"
archived = true

[[rule]]
bundle = ["term"]
title = '^tock │'
project = "tock"

[[rule]]
bundle = ["term"]
category = "Code"

[[rule]]
url = '^(www\.)?youtube\.com(/|$)'
category = "Entertainment"
`

// DaySpans is a typical day (local time):
//
//	09:00-10:40 term "tock │ ✳ x"      Code / tock   1h40
//	10:40-11:14 term "ATLAS │ y"        Code          34m
//	11:14-11:26 arc  youtube           Entertainment 12m
//	11:26-11:35 arc  chatgpt.com/c/1   unlabelled     9m
//	11:35-11:41 arc  labs.x.tv/lab/1   unlabelled     6m
//	11:41-12:22 idle                                  41m
//	12:22-12:25 arc  chatgpt.com/c/2   unlabelled     3m
//	14:50-open  mail "Inbox"           unlabelled    10m (to 15:00)
func DaySpans() []Span {
	return []Span{
		{1, "09:00", "10:40", "term", "WezTerm", "tock │ ✳ x", "", false, 0},
		{2, "10:40", "11:14", "term", "WezTerm", "ATLAS │ y", "", false, 0},
		{3, "11:14", "11:26", "arc", "Arc", "Video", "www.youtube.com/watch", false, 0},
		{4, "11:26", "11:35", "arc", "Arc", "XOR Equivalence Check", "chatgpt.com/c/1", false, 0},
		{5, "11:35", "11:41", "arc", "Arc", "Low Level Labs", "labs.x.tv/lab/1", false, 0},
		{6, "11:41", "12:22", "tock.idle", "Idle", "", "", false, 0},
		{7, "12:22", "12:25", "arc", "Arc", "Proofs", "chatgpt.com/c/2", false, 0},
		{8, "14:50", "", "mail", "Mail", "Inbox", "", false, 0},
	}
}

// FocusHistory is days past days of one focus hour each: 09:00-10:00 in the
// terminal on a "tock" title (Code, project tock, deep under Rules), on each of
// the days days before the day of Now, with ids from 1001 up.
func FocusHistory(days int) []Span {
	out := make([]Span, 0, days)
	for i := 1; i <= days; i++ {
		out = append(out, Span{ID: int64(1000 + i), Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm",
			Title: "tock │ h", Day: -i})
	}
	return out
}
