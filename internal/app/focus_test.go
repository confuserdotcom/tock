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
		Body: "5 min distracted in your 25 min session, 15 min left.", OfSession: true,
		Category: "Entertainment", RuleRef: "rules.toml#3",
		Sess: focus.Session{ID: s.ID, Kind: s.Kind, StartMS: s.StartMS, PlannedMS: s.PlannedMS, Interruptions: 1,
			AlertedMS: 5 * 60_000, DeepMS: 5 * 60_000, DistractMS: 5 * 60_000, LongestMS: 5 * 60_000}}}
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
	ended := focus.Session{ID: 1, Kind: focus.KindManual, StartMS: apptest.At(t, "14:50", 0),
		PlannedMS: apptest.At(t, "15:15", 0), EndMS: apptest.At(t, "15:15", 0),
		DeepMS: 5 * 60_000, DistractMS: 20 * 60_000, LongestMS: 5 * 60_000, Switches: 1}
	if q := focus.Quality(ended, focus.Weights{}); q.Score != 36 {
		t.Fatalf("quality = %d, want 36", q.Score)
	}
	want := []focus.Event{{Kind: focus.Notify, Topic: focus.TopicDone, Title: focus.TitleDone,
		Body:      "25 min · quality 36 · 20% deep · no interruptions · top: Entertainment, Code",
		OfSession: true, Minutes: 25, Project: "tock", Refile: true, Sess: ended}}
	if !reflect.DeepEqual(notes, want) {
		t.Fatalf("notes = %+v, want %+v", notes, want)
	}
	f, err := a.Focus(e.Clock)
	if err != nil || f.Open != nil || len(f.Today) != 1 {
		t.Fatalf("Focus = %+v, %v", f, err)
	}
	if got := f.Today[0]; got != ended {
		t.Fatalf("closed session = %+v, want %+v", got, ended)
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
	// The recap is the only notice: an auto session sends no done notice.
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
		s.EndMS != apptest.At(t, "11:14", 0) || s.DeepMS != 134*60_000 || s.Switches != 0 {
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
	if s.EndMS != apptest.Now.UnixMilli() || s.DeepMS != 5*60_000 || s.DistractMS != 5*60_000 || s.LongestMS != 5*60_000 || s.Switches != 1 {
		t.Fatalf("stopped = %+v", s)
	}
	if f, err := a.Focus(e.Clock); err != nil || len(f.Today) != 1 || f.Today[0] != s {
		t.Fatalf("stored session = %+v, %v; want %+v", f.Today, err, s)
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

func TestGuardTickHooksAndBlock(t *testing.T) {
	e := distracted(t)
	e.WriteConfig(t, "[focus]\nblock = true\n")
	a := open(t, e)
	startAt(t, e, a, "14:50", 25)
	e.Clock = apptest.Now
	_, g, err := a.GuardTick(false)
	if err != nil || g.Start == nil || g.Stop {
		t.Fatalf("first tick = %+v, %v", g, err)
	}
	if g.Block == nil || g.Block.Host != "www.youtube.com" || g.Block.Bundle != "arc" {
		t.Fatalf("block = %+v", g.Block)
	}
	b := open(t, e) // trackd restarted: hooks and the block are remembered
	if _, g, err := b.GuardTick(false); err != nil || g.Start != nil || g.Block != nil {
		t.Fatalf("after restart = %+v, %v", g, err)
	}
	if _, err := b.StopSession(); err != nil {
		t.Fatal(err)
	}
	if _, g, err := b.GuardTick(false); err != nil || !g.Stop {
		t.Fatalf("after stop = %+v, %v", g, err)
	}
}

// seedRated stores rated auto sessions number lo to hi-1 on earlier days whose
// ratings follow the longest run, so the fit moves away from the defaults.
func seedRated(t *testing.T, a *App, lo, hi int) {
	t.Helper()
	for i := lo; i < hi; i++ {
		start := apptest.At(t, "09:00", -(i + 1))
		x := store.Session{Kind: store.SessionAuto, StartMS: start, EndMS: start + 50*60_000,
			DeepMS: int64(10+i%3) * 60_000, LongestMS: int64(5+8*(i%5)) * 60_000, Switches: i % 4}
		if ok, err := a.st.AddAutoSession(x); err != nil || !ok {
			t.Fatalf("AddAutoSession = %v, %v", ok, err)
		}
		ss, err := a.st.Sessions(start, start+1)
		if err != nil || len(ss) != 1 {
			t.Fatalf("Sessions = %v, %v", ss, err)
		}
		if _, _, err := a.RateSession(ss[0].ID, 1+i%5, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFocusTickUsesFittedWeights(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	seedRated(t, a, 0, 10)
	w, n, fitted, err := a.Weights()
	if err != nil || n != 10 || !fitted || w == focus.DefaultQualityWeights() {
		t.Fatalf("Weights = %+v, %d, %v, %v", w, n, fitted, err)
	}
	startAt(t, e, a, "14:50", 25)
	e.Clock = time.UnixMilli(apptest.At(t, "15:16", 0)).In(apptest.Zone)
	notes, err := a.FocusTick(false)
	if err != nil || len(notes) != 1 || notes[0].Topic != focus.TopicDone {
		t.Fatalf("notes = %+v, %v", notes, err)
	}
	ended := notes[0].Sess
	got, def := focus.Quality(ended, w).Score, focus.Quality(ended, focus.Weights{}).Score
	if got == def {
		t.Fatalf("fitted score %d equals the default score; the fixture does not exercise the weights", got)
	}
	if want := "quality " + strconv.Itoa(got) + " "; !strings.Contains(notes[0].Body, want) {
		t.Fatalf("body %q lacks %q (default score %d)", notes[0].Body, want, def)
	}
}

func TestFocusCarriesWeights(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	check := func(wantFitted bool) {
		t.Helper()
		w, _, fitted, err := a.Weights()
		if err != nil || fitted != wantFitted {
			t.Fatalf("Weights = %+v, fitted %v, %v", w, fitted, err)
		}
		f, err := a.Focus(e.Clock)
		if err != nil || f.Weights != w {
			t.Fatalf("Focus.Weights = %+v, want %+v (%v)", f.Weights, w, err)
		}
		if wantFitted == (w == focus.DefaultQualityWeights()) {
			t.Fatalf("weights %+v: default-ness does not match fitted = %v", w, wantFitted)
		}
	}
	check(false)
	seedRated(t, a, 0, 9)
	check(false)
	seedRated(t, a, 9, 10)
	check(true)
}
