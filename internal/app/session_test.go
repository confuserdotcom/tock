package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/focus"
	"tock/internal/store"
)

// sessionEnv is an environment with some recorded time, so Open succeeds.
func sessionEnv(t *testing.T) *apptest.Env {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "14:50", End: "14:55", Bundle: "term", App: "WezTerm", Title: "tock │ a"})
	return e
}

// finishedAt stores a closed manual session from hh:mm for n minutes.
func finishedAt(t *testing.T, a *App, hhmm string, n int, deepMin int) store.Session {
	t.Helper()
	start := apptest.At(t, hhmm, 0)
	x, err := a.st.StartSession(start, start+int64(n)*60_000)
	if err != nil {
		t.Fatal(err)
	}
	x.EndMS = start + int64(n)*60_000
	x.DeepMS = int64(deepMin) * 60_000
	x.LongestMS = x.DeepMS / 2
	x.Switches = 2
	if ok, err := a.st.EndSession(x); err != nil || !ok {
		t.Fatalf("EndSession = %v, %v", ok, err)
	}
	return x
}

func TestRateSession(t *testing.T) {
	e := sessionEnv(t)
	a := open(t, e)
	x := finishedAt(t, a, "09:00", 50, 40)
	for _, bad := range []int{0, 6, -1} {
		if _, _, err := a.RateSession(x.ID, bad, nil); err == nil || err.Error() != "rating must be 1 to 5" {
			t.Fatalf("rating %d: %v", bad, err)
		}
	}
	e.Clock = time.UnixMilli(apptest.At(t, "12:00", 0)).In(apptest.Zone)
	note := "line one\nline two"
	s, prev, err := a.RateSession(x.ID, 1, &note)
	if err != nil || prev != 0 || s.Rating != 1 || s.RatedMS != e.Clock.UnixMilli() || s.Note != "line one line two" {
		t.Fatalf("first rate = %+v, %d, %v", s, prev, err)
	}
	s, prev, err = a.RateSession(x.ID, 5, nil)
	if err != nil || prev != 1 || s.Rating != 5 || s.Note != "line one line two" {
		t.Fatalf("re-rate = %+v, %d, %v", s, prev, err)
	}
	long := strings.Repeat("é", 250)
	s, _, err = a.RateSession(x.ID, 5, &long)
	if err != nil || len([]rune(s.Note)) != 200 {
		t.Fatalf("long note = %d runes, %v", len([]rune(s.Note)), err)
	}
	open1, err := a.st.StartSession(apptest.At(t, "13:00", 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.RateSession(open1.ID, 3, nil); !errors.Is(err, store.ErrSessionOpen) {
		t.Fatalf("running: %v", err)
	}
	if _, _, err := a.RateSession(999, 3, nil); !errors.Is(err, store.ErrSessionGone) {
		t.Fatalf("unknown: %v", err)
	}
	if got, err := a.LastSession(); err != nil || got == nil || got.ID != x.ID {
		t.Fatalf("LastSession = %+v, %v", got, err)
	}
}

func TestSetNote(t *testing.T) {
	e := sessionEnv(t)
	a := open(t, e)
	x := finishedAt(t, a, "09:00", 25, 20)
	s, err := a.SetNote(x.ID, "a\tb\x1bc")
	if err != nil || s.Note != "a b?c" {
		t.Fatalf("SetNote = %+v, %v", s, err)
	}
	if s, err := a.SetNote(x.ID, "   "); err != nil || s.Note != "" {
		t.Fatalf("clear = %+v, %v", s, err)
	}
	if _, err := a.SetNote(999, "x"); !errors.Is(err, store.ErrSessionGone) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestSessionWeightsApp(t *testing.T) {
	e := sessionEnv(t)
	a := open(t, e)
	for i := 0; i < 10; i++ {
		x := finishedAt(t, a, []string{"00:10", "01:10", "02:10", "03:10", "04:10", "05:10", "06:10", "07:10", "08:10", "09:10"}[i], 50, 5+i*4)
		if _, _, err := a.RateSession(x.ID, 1+i%5, nil); err != nil {
			t.Fatal(err)
		}
		w, n, fitted, err := a.Weights()
		if err != nil {
			t.Fatal(err)
		}
		if i < 9 && (fitted || n != i+1 || w != focus.DefaultQualityWeights()) {
			t.Fatalf("after %d ratings: %v %d %v", i+1, w, n, fitted)
		}
		if i == 9 {
			ss, _ := a.st.RatedSessions()
			want, wn := focus.FitWeights(focusSessions(ss))
			if !fitted || n != 10 || w != want || wn != 10 {
				t.Fatalf("after 10: %v %d %v, want %v", w, n, fitted, want)
			}
		}
	}
}

func TestSessionDetail(t *testing.T) {
	_, a, x := refileEnv(t)
	d, err := a.SessionDetail(x.ID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := a.SessionByID(x.ID)
	if err != nil || d.Sess != s {
		t.Fatalf("Sess = %+v, want %+v (%v)", d.Sess, s, err)
	}
	w, _, _, _ := a.Weights()
	if want := focus.Quality(s, w); d.Q != want {
		t.Fatalf("Q = %+v, want %+v", d.Q, want)
	}
	vs, labels, err := a.labelled(s.StartMS, s.EndMS, a.nowMS())
	if err != nil {
		t.Fatal(err)
	}
	if want := focus.Window(vs, labels, s.StartMS, s.EndMS, a.cfg.Focus); !reflect.DeepEqual(d.Top, want) || d.Top.ActiveMS != 40*60_000 {
		t.Fatalf("Top = %+v, want %+v", d.Top, want)
	}
	run, err := a.st.StartSession(apptest.At(t, "13:00", 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SessionDetail(run.ID); !errors.Is(err, store.ErrSessionOpen) {
		t.Fatalf("running: %v", err)
	}
	if _, err := a.SessionDetail(999); !errors.Is(err, store.ErrSessionGone) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestSessionDetailAfterRefile(t *testing.T) {
	_, a, x := refileEnv(t)
	d, err := a.SessionDetail(x.ID)
	if err != nil || d.Top.Project != "tock" {
		t.Fatalf("before: %+v, %v", d.Top, err)
	}
	if _, _, err := a.RefileSession(x.ID, "Analyse 1"); err != nil {
		t.Fatal(err)
	}
	d, err = a.SessionDetail(x.ID)
	if err != nil || d.Top.Project != "Analyse 1" || d.Top.Refile() {
		t.Fatalf("after: %+v, %v", d.Top, err)
	}
}
