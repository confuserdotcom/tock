package app

import (
	"errors"
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

// distracted seeds 5 min of Code then YouTube, open, from 14:55.
func distracted(t *testing.T) *apptest.Env {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "14:50", End: "14:55", Bundle: "term", App: "WezTerm", Title: "tock │ a"},
		apptest.Span{ID: 2, Start: "14:55", Bundle: "arc", App: "Arc", Title: "Video", URL: "www.youtube.com/watch"},
	)
	return e
}

// startAt starts a session of n minutes at hh:mm today.
func startAt(t *testing.T, e *apptest.Env, a *App, hhmm string, n int) focus.Session {
	t.Helper()
	e.Clock = time.UnixMilli(apptest.At(t, hhmm, 0)).In(apptest.Zone)
	s, err := a.StartSession(n)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFocusTickAlertsOnceAcrossRestart(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	s := startAt(t, e, a, "14:50", 25)
	if s.PlannedMS != apptest.At(t, "15:15", 0) {
		t.Fatalf("planned = %d", s.PlannedMS)
	}
	e.Clock = apptest.Now
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	want := []focus.Event{{Kind: focus.Notify, Topic: focus.TopicFocus, Title: focus.TitleFocus,
		Body: "5 min distracted in your 25 min session, 15 min left.", OfSession: true}}
	if !reflect.DeepEqual(notes, want) {
		t.Fatalf("notes = %+v, want %+v", notes, want)
	}
	for _, n := range notes {
		if strings.Contains(n.Body, "Video") || strings.Contains(n.Body, "youtube") {
			t.Fatalf("notification leaks a title or URL: %q", n.Body)
		}
	}
	if notes, err := a.FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("second tick = %+v, %v", notes, err)
	}
	b := open(t, e) // trackd restarted
	if notes, err := b.FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("tick after restart = %+v, %v", notes, err)
	}
	f, err := b.Focus(e.Clock)
	if err != nil || f.Open == nil || f.Open.Interruptions != 1 {
		t.Fatalf("Focus = %+v, %v", f, err)
	}
}

func TestFocusTickEndsSession(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	startAt(t, e, a, "14:50", 25)
	e.Clock = time.UnixMilli(apptest.At(t, "15:16", 0)).In(apptest.Zone)
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	want := []focus.Event{{Kind: focus.Notify, Topic: focus.TopicDone, Title: focus.TitleDone,
		Body: "25 min: 20% deep, 0 interruptions, longest 5 min. Take a 5 min break.", OfSession: true, Minutes: 25}}
	if !reflect.DeepEqual(notes, want) {
		t.Fatalf("notes = %+v, want %+v", notes, want)
	}
	f, err := a.Focus(e.Clock)
	if err != nil || f.Open != nil || len(f.Today) != 1 {
		t.Fatalf("Focus = %+v, %v", f, err)
	}
	if got := f.Today[0]; got.EndMS != apptest.At(t, "15:15", 0) || got.DistractMS != 20*60_000 {
		t.Fatalf("closed session = %+v", got)
	}
	if v, _, _ := a.st.State(store.KeyFocusBreakAt); v != strconv.FormatInt(e.Clock.UnixMilli(), 10) {
		t.Fatalf("focus_break_at = %q, want the session end tick", v)
	}
}

func TestFocusTickAutoSessionAndRecap(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.WriteConfig(t, "[focus]\nrecap = \"14:00\"\n")
	a := open(t, e)
	day, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	want := []focus.Event{{Kind: focus.Notify, Topic: focus.TopicRecap, Title: focus.TitleRecap, Body: report.Recap(day)}}
	if !reflect.DeepEqual(notes, want) || !strings.HasSuffix(want[0].Body, "Top project: tock.") {
		t.Fatalf("notes = %+v, want %+v", notes, want)
	}
	if notes, err := a.FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("second tick = %+v, %v", notes, err)
	}
	f, err := a.Focus(e.Clock)
	if err != nil || len(f.Today) != 1 {
		t.Fatalf("Focus = %+v, %v", f, err)
	}
	if s := f.Today[0]; s.Kind != focus.KindAuto || s.StartMS != apptest.At(t, "09:00", 0) ||
		s.EndMS != apptest.At(t, "11:14", 0) || s.DeepMS != 134*60_000 {
		t.Fatalf("auto session = %+v", s)
	}
}

func TestStartStopSession(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	startAt(t, e, a, "14:50", 25)
	if _, err := a.StartSession(25); !errors.Is(err, store.ErrSessionRunning) {
		t.Fatalf("second start: %v", err)
	}
	for _, n := range []int{0, 241} {
		if _, err := a.StartSession(n); err == nil || err.Error() != "session length "+strconv.Itoa(n)+" min, want 1 to 240" {
			t.Fatalf("StartSession(%d) err = %v", n, err)
		}
	}
	e.Clock = apptest.Now
	s, err := a.StopSession()
	if err != nil {
		t.Fatal(err)
	}
	if s.EndMS != apptest.Now.UnixMilli() || s.DeepMS != 5*60_000 || s.DistractMS != 5*60_000 || s.LongestMS != 5*60_000 {
		t.Fatalf("stopped = %+v", s)
	}
	if _, err := a.StopSession(); !errors.Is(err, store.ErrNoSession) {
		t.Fatalf("second stop: %v", err)
	}
}

func TestFocusHistory(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	if _, err := a.st.AddAutoSession(store.Session{Kind: store.SessionAuto,
		StartMS: apptest.At(t, "09:00", -1), EndMS: apptest.At(t, "10:00", -1), DeepMS: 3_600_000}); err != nil {
		t.Fatal(err)
	}
	startAt(t, e, a, "14:50", 25)
	e.Clock = apptest.Now
	f, err := a.Focus(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if f.Open == nil || f.Open.DeepMS != 5*60_000 || f.Open.DistractMS != 5*60_000 || len(f.Today) != 0 || len(f.Week) != 1 {
		t.Fatalf("Focus = %+v", f)
	}
}

func TestStatusShowsSession(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Heartbeat(t, e.Clock.UnixMilli())
	a := open(t, e)
	if _, err := a.StartSession(25); err != nil {
		t.Fatal(err)
	}
	line, err := a.Status()
	if err != nil || !strings.HasSuffix(line, " · ◷ 25m") {
		t.Fatalf("Status = %q, %v", line, err)
	}
}

func TestSnoozeBreak(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "14:00", Bundle: "term", App: "WezTerm", Title: "tock │ a"})
	a := open(t, e)
	e.Clock = time.UnixMilli(apptest.At(t, "14:40", 0)).In(apptest.Zone)
	if err := a.SnoozeBreak(30 * time.Minute); err != nil {
		t.Fatal(err)
	}
	for _, hhmm := range []string{"14:51", "15:09"} {
		e.Clock = time.UnixMilli(apptest.At(t, hhmm, 0)).In(apptest.Zone)
		if notes, err := a.FocusTick(false); err != nil || len(notes) != 0 {
			t.Fatalf("%s: snoozed reminder = %+v, %v", hhmm, notes, err)
		}
	}
	e.Clock = time.UnixMilli(apptest.At(t, "15:10", 0)).In(apptest.Zone)
	notes, err := a.FocusTick(false)
	if err != nil || len(notes) != 1 || notes[0].Topic != focus.TopicBreak {
		t.Fatalf("after the snooze = %+v, %v", notes, err)
	}
}
