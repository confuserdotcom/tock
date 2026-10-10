package main

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/collect"
	"tock/internal/config"
	"tock/internal/store"
)

type fakeCal struct {
	status   string
	grant    string // what Request turns status into
	requests int
	reads    int
	cals     []string
	evs      []collect.CalendarEvent
	err      error
}

func (f *fakeCal) Status() string { return f.status }
func (f *fakeCal) Request() string {
	f.requests++
	f.status = f.grant
	return f.status
}
func (f *fakeCal) Events(from, to int64, cals []string) ([]collect.CalendarEvent, error) {
	f.reads++
	f.cals = cals
	return f.evs, f.err
}

func calLoop(t *testing.T, cal Calendar, prompt bool, cfg string) (*calendarLoop, *store.Store, *bytes.Buffer, *int64) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := filepath.Join(dir, config.File)
	if err := os.WriteFile(p, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	now := int64(1_800_000_000_000)
	var logb bytes.Buffer
	c := &calendarLoop{cal: cal, st: st, cfgPath: p, canPrompt: prompt,
		log: slog.New(slog.NewTextHandler(&logb, nil)), now: func() int64 { return now }}
	return c, st, &logb, &now
}

func state(t *testing.T, st *store.Store, key string) string {
	t.Helper()
	m, err := st.States()
	if err != nil {
		t.Fatal(err)
	}
	return m[key]
}

func TestCalendarLoopSyncs(t *testing.T) {
	f := &fakeCal{status: collect.CalendarUndetermined, grant: collect.CalendarGranted}
	c, st, logb, now := calLoop(t, f, true, "[calendar]\nenabled = true\ncalendars = [\"Work\"]\n")
	f.evs = []collect.CalendarEvent{{StartMS: *now - 3600_000, EndMS: *now - 1800_000, Title: "Secret standup", Calendar: "Work"}}

	c.tick()
	if f.requests != 1 || f.reads != 1 || len(f.cals) != 1 || f.cals[0] != "Work" {
		t.Fatalf("requests %d reads %d cals %v", f.requests, f.reads, f.cals)
	}
	if state(t, st, store.KeyCalendarStatus) != "granted" || state(t, st, store.KeyCalendarSync) == "" {
		t.Fatalf("states: %q %q", state(t, st, store.KeyCalendarStatus), state(t, st, store.KeyCalendarSync))
	}
	if evs, _ := st.Events(0, *now); len(evs) != 1 || evs[0].Title != "Secret standup" {
		t.Fatalf("events = %+v", evs)
	}
	// Not due yet: no read. Due after 10 min.
	*now += 60_000
	c.tick()
	if f.reads != 1 {
		t.Fatalf("read again after 1 min: %d", f.reads)
	}
	*now += calendarSyncEvery
	c.tick()
	if f.reads != 2 || f.requests != 1 {
		t.Fatalf("reads %d requests %d", f.reads, f.requests)
	}
	if strings.Contains(logb.String(), "Secret") || strings.Contains(logb.String(), "Work") {
		t.Fatalf("log leaks event data:\n%s", logb)
	}
	// A read error is recorded and logged once.
	f.err = errors.New("boom")
	*now += calendarSyncEvery
	c.tick()
	c.tick()
	if state(t, st, store.KeyCalendarStatus) != "error" || strings.Count(logb.String(), "read failed") != 1 {
		t.Fatalf("status %q, log:\n%s", state(t, st, store.KeyCalendarStatus), logb)
	}
}

func TestCalendarLoopOffAndPrompt(t *testing.T) {
	f := &fakeCal{status: collect.CalendarUndetermined, grant: collect.CalendarDenied}
	c, st, logb, _ := calLoop(t, f, true, "")
	c.tick()
	if f.requests != 0 || f.reads != 0 || state(t, st, store.KeyCalendarStatus) != "off" {
		t.Fatalf("disabled: requests %d reads %d status %q", f.requests, f.reads, state(t, st, store.KeyCalendarStatus))
	}

	// Enabled outside launchd: never prompt.
	f2 := &fakeCal{status: collect.CalendarUndetermined, grant: collect.CalendarGranted}
	c2, st2, _, _ := calLoop(t, f2, false, "[calendar]\nenabled = true\n")
	c2.tick()
	if f2.requests != 0 || f2.reads != 0 || state(t, st2, store.KeyCalendarStatus) != "undetermined" {
		t.Fatalf("no prompt outside launchd: requests %d status %q", f2.requests, state(t, st2, store.KeyCalendarStatus))
	}

	// Under launchd: ask once, even when denied.
	if err := os.WriteFile(c.cfgPath, []byte("[calendar]\nenabled = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.tick()
	c.tick()
	if f.requests != 1 || f.reads != 0 || state(t, st, store.KeyCalendarStatus) != "denied" ||
		!strings.Contains(logb.String(), "Privacy & Security > Calendars") {
		t.Fatalf("denied: requests %d reads %d status %q\n%s", f.requests, f.reads, state(t, st, store.KeyCalendarStatus), logb)
	}
}
