package app

import (
	"testing"

	"tock/internal/apptest"
	"tock/internal/focus"
)

// breakEnv is a Saturday with days past days of a 52 min terminal run each
// (08:00-08:52), an open terminal span since 09:00 today, no [hours] (so no
// focus suggestions), no routine or plan notices, and the clock at 09:30.
func breakEnv(t *testing.T, days int, extra string) *apptest.Env {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	var hist []apptest.Span
	for i := 1; i <= days; i++ {
		hist = append(hist, apptest.Span{ID: int64(2000 + i), Start: "08:00", End: "08:52", Bundle: "term", App: "WezTerm",
			Title: "tock │ h", Day: -i})
	}
	e.Seed(t, hist...)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices+extra)
	slotClock(e, 9, 30, 0)
	return e
}

// breakTick ticks at h:m:s and returns the break reminders it produced.
func breakTick(t *testing.T, a *App, e *apptest.Env, h, m, s int) []focus.Event {
	t.Helper()
	slotClock(e, h, m, s)
	evs, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	var out []focus.Event
	for _, ev := range evs {
		if ev.Topic == focus.TopicBreak {
			out = append(out, ev)
		}
	}
	return out
}

func wantBreak(t *testing.T, label string, got []focus.Event, body string) {
	t.Helper()
	if body == "" {
		if len(got) != 0 {
			t.Fatalf("%s: got %+v, want no break reminder", label, got)
		}
		return
	}
	if len(got) != 1 || got[0].Title != focus.TitleBreak || got[0].Body != body {
		t.Fatalf("%s: got %+v, want one break reminder %q", label, got, body)
	}
}

func TestAdaptiveBreakFromHistory(t *testing.T) {
	e := breakEnv(t, 10, "")
	a := open(t, e)
	wantBreak(t, "09:51", breakTick(t, a, e, 9, 51, 0), "")
	wantBreak(t, "09:52", breakTick(t, a, e, 9, 52, 0), "52 min without a break. Stand up for 10 min.")

	// adaptive_break off: the [focus] break_after of 50 min.
	e = breakEnv(t, 10, "[recommend]\nadaptive_break = false\n")
	a = open(t, e)
	wantBreak(t, "off 09:50", breakTick(t, a, e, 9, 50, 0), "50 min without a break. Stand up for 5 min.")

	// Five days is too little history for a sustained length.
	e = breakEnv(t, 5, "")
	a = open(t, e)
	wantBreak(t, "thin 09:50", breakTick(t, a, e, 9, 50, 0), "50 min without a break. Stand up for 5 min.")
}
