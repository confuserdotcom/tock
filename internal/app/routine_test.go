package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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

func TestStartOfDayPlan(t *testing.T) {
	file := filepath.Join(t.TempDir(), "todo.txt")
	if err := os.WriteFile(file, []byte("Standup due:2026-10-10 @10h00\nReview due:2026-10-10 @14:00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := startEnv(t, "[plan]\nfile = '"+file+"'\n")
	a := open(t, e)
	notes, err := a.FocusTick(false)
	if err != nil || len(notes) != 1 || !strings.HasSuffix(notes[0].Body, "Today: 10:00 Standup · 14:00 Review.") {
		t.Fatalf("notes = %+v, %v", notes, err)
	}

	e = startEnv(t, "[plan]\nfile = \""+filepath.Join(t.TempDir(), "missing", "todo.txt")+"\"\n")
	a = open(t, e)
	notes, err = a.FocusTick(false)
	if err != nil || len(notes) != 1 || strings.Contains(notes[0].Body, "Today") {
		t.Fatalf("missing plan: notes = %+v, %v", notes, err)
	}
}

// middayEnv seeds an open span from 12:30 with the clock at 13:00, so the
// midday check-in is due.
func middayEnv(t *testing.T, config string) *apptest.Env {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "12:30", Bundle: "term", App: "WezTerm", Title: "secret-today-title"})
	e.WriteConfig(t, config)
	e.Clock = time.Date(2026, 10, 10, 13, 0, 0, 0, apptest.Zone)
	return e
}

func TestMiddayCheckIn(t *testing.T) {
	e := middayEnv(t, "[routine]\nstart = false\n")
	a := open(t, e)
	d, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	body := report.MiddayBody(d)
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	want := []focus.Event{{Kind: focus.Notify, Topic: focus.TopicMidday, Title: focus.TitleMidday, Body: body}}
	if !reflect.DeepEqual(notes, want) {
		t.Fatalf("notes = %+v, want %+v", notes, want)
	}
	if strings.Contains(body, "secret") {
		t.Fatalf("body = %q", body)
	}
	if notes, err := a.FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("second tick = %+v, %v", notes, err)
	}
	b := open(t, e)
	if notes, err := b.FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("tick after restart = %+v, %v", notes, err)
	}
	if v, _, _ := a.st.State(store.KeyFocusMiddayDay); v != "2026-10-10" {
		t.Fatalf("focus_midday_day = %q", v)
	}
	if got, err := a.MiddayBody(e.Clock); err != nil || got != body {
		t.Fatalf("MiddayBody = %q, %v", got, err)
	}
}

func TestMiddaySkippedAfterHours(t *testing.T) {
	e := middayEnv(t, "[focus]\nrecap = \"\"\n[hours]\ndays = [\"mon\",\"tue\",\"wed\",\"thu\",\"fri\",\"sat\",\"sun\"]\nstart = \"09:00\"\nend = \"12:00\"\n[routine]\nstart = false\noverwork = false\n")
	a := open(t, e)
	if notes, err := a.FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("notes = %+v, %v", notes, err)
	}
}

func TestRoutineEndOff(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.WriteConfig(t, "[focus]\nrecap = \"14:00\"\n[routine]\nstart = false\nmidday = \"off\"\nend = false\n")
	a := open(t, e)
	day, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := a.FocusTick(false)
	if err != nil || len(notes) != 0 {
		t.Fatalf("notes = %+v, %v", notes, err)
	}
	f, err := a.Focus(e.Clock)
	if err != nil || len(f.Today) != 1 {
		t.Fatalf("auto session not recorded: %+v, %v", f, err)
	}
	text, err := a.Recap(e.Clock)
	if err != nil || text != report.Recap(day) || !strings.HasSuffix(text, "Top project: tock.") {
		t.Fatalf("Recap = %q, %v", text, err)
	}
}

// overEnv seeds an open Code span from 13:00 with the hours ending at 14:00;
// the clock is set by the test.
func overEnv(t *testing.T, hoursExtra string) *apptest.Env {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "13:00", Bundle: "term", App: "WezTerm", Title: "secret-today-title"})
	e.WriteConfig(t, "[focus]\nbreak_after = 480\n"+apptest.NoNudgeLimit+"[hours]\ndays = [\"mon\",\"tue\",\"wed\",\"thu\",\"fri\",\"sat\",\"sun\"]\nstart = \"09:00\"\nend = \"14:00\"\n"+
		hoursExtra+"[routine]\nstart = false\nmidday = \"off\"\nend = false\n")
	return e
}

func overTick(t *testing.T, a *App, e *apptest.Env, h, m int) []focus.Event {
	t.Helper()
	e.Clock = time.Date(2026, 10, 10, h, m, 0, 0, apptest.Zone)
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	return notes
}

func TestOverworking(t *testing.T) {
	e := overEnv(t, "")
	a := open(t, e)
	if notes := overTick(t, a, e, 14, 29); len(notes) != 0 {
		t.Fatalf("14:29 notes = %+v", notes)
	}
	notes := overTick(t, a, e, 14, 30)
	d, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	body := report.OverBody(d, 30*60_000)
	want := []focus.Event{{Kind: focus.Notify, Topic: focus.TopicOver, Title: focus.TitleOver, Body: body}}
	if !reflect.DeepEqual(notes, want) {
		t.Fatalf("14:30 notes = %+v, want %+v", notes, want)
	}
	if !strings.Contains(body, "after 14:00") || strings.Contains(body, "secret") {
		t.Fatalf("body = %q", body)
	}
	if notes := overTick(t, a, e, 14, 30); len(notes) != 0 {
		t.Fatalf("second tick = %+v", notes)
	}
	b := open(t, e)
	if notes := overTick(t, b, e, 14, 31); len(notes) != 0 {
		t.Fatalf("tick after restart = %+v", notes)
	}
	if notes := overTick(t, b, e, 15, 29); len(notes) != 0 {
		t.Fatalf("15:29 notes = %+v", notes)
	}
	if notes := overTick(t, b, e, 15, 30); len(notes) != 1 || notes[0].Topic != focus.TopicOver {
		t.Fatalf("15:30 notes = %+v", notes)
	}
	if v, _, _ := b.st.State(store.KeyFocusOverDay); v != "2026-10-10" {
		t.Fatalf("focus_over_day = %q", v)
	}
	if v, _, _ := b.st.State(store.KeyFocusOverAt); v != strconv.FormatInt(e.Clock.UnixMilli(), 10) {
		t.Fatalf("focus_over_at = %q, want %d", v, e.Clock.UnixMilli())
	}
}

func TestOverworkingStoppedBeforeGrace(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "13:00", End: "14:20", Bundle: "term", App: "WezTerm", Title: "t"},
		apptest.Span{ID: 2, Start: "14:20", Bundle: "tock.idle", App: "idle", Title: ""},
	)
	e.WriteConfig(t, "[focus]\nbreak_after = 480\n[hours]\ndays = [\"mon\",\"tue\",\"wed\",\"thu\",\"fri\",\"sat\",\"sun\"]\nstart = \"09:00\"\nend = \"14:00\"\n[routine]\nstart = false\nmidday = \"off\"\nend = false\n")
	a := open(t, e)
	if notes := overTick(t, a, e, 15, 0); len(notes) != 0 {
		t.Fatalf("notes = %+v", notes)
	}
}

func TestOverworkingOff(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "13:00", Bundle: "term", App: "WezTerm", Title: "t"})
	e.WriteConfig(t, "[focus]\nbreak_after = 480\n[hours]\ndays = [\"mon\",\"tue\",\"wed\",\"thu\",\"fri\",\"sat\",\"sun\"]\nstart = \"09:00\"\nend = \"14:00\"\n[routine]\nstart = false\nmidday = \"off\"\nend = false\noverwork = false\n")
	a := open(t, e)
	if notes := overTick(t, a, e, 14, 45); len(notes) != 0 {
		t.Fatalf("notes = %+v", notes)
	}
}

func TestOverworkingCustomTiming(t *testing.T) {
	e := overEnv(t, "over_grace = 10\nover_repeat = 15\n")
	a := open(t, e)
	for _, tt := range []struct {
		h, m int
		want int
	}{{14, 9, 0}, {14, 10, 1}, {14, 24, 0}, {14, 25, 1}} {
		if notes := overTick(t, a, e, tt.h, tt.m); len(notes) != tt.want {
			t.Fatalf("%02d:%02d notes = %+v, want %d", tt.h, tt.m, notes, tt.want)
		}
	}
}

// TestRoutineWorkDay runs one work day through the focus tick and checks that
// the four routine notices come in order, once each, even across a restart.
func TestRoutineWorkDay(t *testing.T) {
	file := filepath.Join(t.TempDir(), "todo.txt")
	if err := os.WriteFile(file, []byte("Standup due:2026-10-10 @10h00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "12:00", Bundle: "term", App: "WezTerm", Title: "yesterday", Day: -1},
		apptest.Span{ID: 2, Start: "09:00", End: "12:00", Bundle: "term", App: "WezTerm", Title: "morning"},
		apptest.Span{ID: 3, Start: "12:00", End: "12:30", Bundle: "tock.idle", App: "Idle"},
		apptest.Span{ID: 4, Start: "12:30", Bundle: "term", App: "WezTerm", Title: "afternoon"},
	)
	e.WriteConfig(t, "[focus]\nbreak_after = 480\n[hours]\ndays = [\"mon\",\"tue\",\"wed\",\"thu\",\"fri\",\"sat\",\"sun\"]\nstart = \"09:00\"\nend = \"17:00\"\n"+
		"[plan]\nfile = '"+file+"'\n")
	a := open(t, e)
	tick := func(a *App, h, m int) []string {
		t.Helper()
		e.Clock = time.Date(2026, 10, 10, h, m, 0, 0, apptest.Zone)
		notes, err := a.FocusTick(false)
		if err != nil {
			t.Fatal(err)
		}
		topics := []string{}
		for _, n := range notes {
			topics = append(topics, n.Topic)
		}
		if h == 9 && m == 5 && (len(notes) != 1 || !strings.HasSuffix(notes[0].Body, "Today: 10:00 Standup.")) {
			t.Fatalf("start notice = %+v", notes)
		}
		return topics
	}
	steps := []struct {
		h, m int
		want []string
	}{
		{9, 5, []string{focus.TopicStart}},
		{12, 59, []string{}},
		{13, 0, []string{focus.TopicMidday}},
		{16, 59, []string{}},
		{17, 0, []string{focus.TopicRecap}},
		{17, 29, []string{}},
		{17, 30, []string{focus.TopicOver}},
		{18, 29, []string{}},
	}
	for _, s := range steps {
		if got := tick(a, s.h, s.m); !reflect.DeepEqual(got, s.want) {
			t.Fatalf("%02d:%02d topics = %v, want %v", s.h, s.m, got, s.want)
		}
	}
	b := open(t, e) // trackd restarts
	for _, s := range []struct {
		h, m int
		want []string
	}{{18, 30, []string{focus.TopicOver}}, {18, 31, []string{}}} {
		if got := tick(b, s.h, s.m); !reflect.DeepEqual(got, s.want) {
			t.Fatalf("after restart %02d:%02d topics = %v, want %v", s.h, s.m, got, s.want)
		}
	}
}

// TestStartOfDayOvernight: work that began yesterday evening and goes on past
// midnight is no start of day, and the notice does not come at 00:01.
func TestStartOfDayOvernight(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "22:00", Bundle: "term", App: "WezTerm", Title: "night", Day: -1})
	e.Clock = time.Date(2026, 10, 10, 0, 30, 0, 0, apptest.Zone)
	a := open(t, e)
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if n.Topic == focus.TopicStart {
			t.Fatalf("start notice on overnight work: %+v", n)
		}
	}
}

// TestRoutineBodyFailure: a routine body that cannot be built skips that
// notice for the tick, is reported through RoutineErr and neither stops the
// focus engine nor marks the notice as sent.
func TestRoutineBodyFailure(t *testing.T) {
	e := startEnv(t, "")
	a := open(t, e)
	db := e.RawDB(t)
	defer db.Close()
	// Yesterday's report reads the summary table.
	if _, err := db.Exec(`ALTER TABLE summary RENAME TO summary_gone`); err != nil {
		t.Fatal(err)
	}
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatalf("tick failed over a routine body: %v", err)
	}
	for _, n := range notes {
		if n.Topic == focus.TopicStart {
			t.Fatalf("start notice without a body: %+v", n)
		}
	}
	if a.RoutineErr() == nil {
		t.Fatal("RoutineErr = nil after a failed body")
	}
	if v, _, _ := a.st.State(store.KeyFocusStartDay); v != "" {
		t.Fatalf("focus_start_day = %q, want unset", v)
	}
	if _, err := db.Exec(`ALTER TABLE summary_gone RENAME TO summary`); err != nil {
		t.Fatal(err)
	}
	notes, err = a.FocusTick(false)
	if err != nil || len(notes) != 1 || notes[0].Topic != focus.TopicStart {
		t.Fatalf("tick after the failure = %+v, %v", notes, err)
	}
	if a.RoutineErr() != nil {
		t.Fatalf("RoutineErr = %v after a good tick", a.RoutineErr())
	}
}
