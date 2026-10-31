package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

// startEnv seeds a closed span yesterday and an open one today from 09:00,
// with the clock at 09:30, so the start-of-day notice is due.
func startEnv(t *testing.T, config string) *apptest.Env {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "secret-yesterday-title", Day: -1},
		apptest.Span{ID: 2, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "secret-today-title"},
	)
	if config != "" {
		e.WriteConfig(t, config)
	}
	e.Clock = time.Date(2026, 10, 10, 9, 30, 0, 0, apptest.Zone)
	return e
}

func TestStartOfDay(t *testing.T) {
	e := startEnv(t, "")
	a := open(t, e)
	y, err := a.Day(e.Clock.AddDate(0, 0, -1))
	if err != nil {
		t.Fatal(err)
	}
	body := report.StartBody(y, nil)
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	want := []focus.Event{{Kind: focus.Notify, Topic: focus.TopicStart, Title: focus.TitleStart, Body: body}}
	if !reflect.DeepEqual(notes, want) {
		t.Fatalf("notes = %+v, want %+v", notes, want)
	}
	if !strings.HasPrefix(body, "Yesterday: ") || strings.Contains(body, "secret") {
		t.Fatalf("body = %q", body)
	}
	if notes, err := a.FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("second tick = %+v, %v", notes, err)
	}
	// trackd restarts: a new App over the same files.
	b := open(t, e)
	if notes, err := b.FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("tick after restart = %+v, %v", notes, err)
	}
	if v, _, _ := a.st.State(store.KeyFocusStartDay); v != "2026-10-10" {
		t.Fatalf("focus_start_day = %q", v)
	}
}

func TestStartOfDayDayOff(t *testing.T) {
	e := startEnv(t, "[hours]\ndays = [\"mon\"]\nstart = \"09:00\"\nend = \"18:00\"\n")
	a := open(t, e)
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if n.Topic == focus.TopicStart {
			t.Fatalf("start notice on a day off: %+v", n)
		}
	}
}

func TestStartOfDayOff(t *testing.T) {
	e := startEnv(t, apptest.NoRoutines)
	a := open(t, e)
	if notes, err := a.FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("notes = %+v, %v", notes, err)
	}
}
