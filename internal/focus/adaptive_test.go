package focus

import (
	"testing"

	"tock/internal/config"
	"tock/internal/recommend"
)

// breakEvents are the break reminders among evs, whatever their kind (a plain
// Notify, or a RecDue once the reminder is a recommendation).
func breakEvents(evs []Event) []Event {
	var out []Event
	for _, e := range evs {
		if (e.Kind == Notify || e.Kind == RecDue) && e.Topic == TopicBreak {
			out = append(out, e)
		}
	}
	return out
}

// adaptiveIn is a tick at now over deep views from 09:00, with the
// recommendations on and a 52 min sustained-focus profile.
func adaptiveIn(now int64) Input {
	in := input(now, vw{from: at(9, 0, 0), to: now, kind: "deep"})
	in.Rec = config.DefaultRecommend()
	in.Profile = recommend.Profile{SustainedMS: 52 * minute}
	return in
}

func TestAdaptiveBreak(t *testing.T) {
	if got := breakEvents(Step(adaptiveIn(at(9, 51, 0)))); len(got) != 0 {
		t.Fatalf("09:51: %+v, want no reminder", got)
	}
	evs := Step(adaptiveIn(at(9, 52, 0)))
	got := breakEvents(evs)
	if len(got) != 1 || got[0].Title != TitleBreak || got[0].Body != "52 min without a break. Stand up for 10 min." {
		t.Fatalf("09:52: %+v, want one 52 min reminder with a 10 min break", got)
	}
	if st := only(evs, StateUpdate); len(st) != 1 || st[0].State.BreakFrom != at(9, 0, 0) || st[0].State.BreakAt != at(9, 52, 0) {
		t.Fatalf("09:52 state = %+v", st)
	}

	in := adaptiveIn(at(10, 2, 0))
	in.State = State{BreakFrom: at(9, 0, 0), BreakAt: at(9, 52, 0)}
	got = breakEvents(Step(in))
	if len(got) != 1 || got[0].Body != "62 min without a break. Stand up for 10 min." {
		t.Fatalf("10:02: %+v, want the 62 min reminder", got)
	}

	// adaptive_break off: the [focus] break_after of 50 min and the 5 min body.
	in = adaptiveIn(at(9, 50, 0))
	in.Rec.AdaptiveBreak = false
	got = breakEvents(Step(in))
	if len(got) != 1 || got[0].Body != "50 min without a break. Stand up for 5 min." {
		t.Fatalf("adaptive_break off: %+v", got)
	}

	// Recommendations disabled, like the zero value of TestBreaks.
	in = adaptiveIn(at(9, 50, 0))
	in.Rec.Enabled = false
	got = breakEvents(Step(in))
	if len(got) != 1 || got[0].Body != "50 min without a break. Stand up for 5 min." {
		t.Fatalf("disabled: %+v", got)
	}

	// A thin profile (no sustained length) keeps the fallback and the 5 min body.
	in = adaptiveIn(at(9, 50, 0))
	in.Profile = recommend.Profile{}
	got = breakEvents(Step(in))
	if len(got) != 1 || got[0].Body != "50 min without a break. Stand up for 5 min." {
		t.Fatalf("thin: %+v", got)
	}

	// A long sustained length is clamped to 120 min.
	in = adaptiveIn(at(10, 59, 0))
	in.Profile.SustainedMS = 150 * minute
	if got = breakEvents(Step(in)); len(got) != 0 {
		t.Fatalf("10:59 with sustained 150: %+v, want none (120 min cap)", got)
	}
	in = adaptiveIn(at(11, 0, 0))
	in.Profile.SustainedMS = 150 * minute
	if got = breakEvents(Step(in)); len(got) != 1 || got[0].Body != "120 min without a break. Stand up for 20 min." {
		t.Fatalf("11:00 with sustained 150: %+v", got)
	}
}

func TestStepBreakRec(t *testing.T) {
	evs := Step(adaptiveIn(at(9, 52, 0)))
	got := breakEvents(evs)
	if len(got) != 1 {
		t.Fatalf("events = %+v", evs)
	}
	e := got[0]
	if e.Kind != RecDue || e.Topic != TopicBreak || e.Title != TitleBreak || e.Body != "52 min without a break. Stand up for 10 min." ||
		e.Minutes != 10 || e.RecKind != recommend.KindBreak || e.RecLengthMS != 10*minute {
		t.Fatalf("event = %+v", e)
	}
	if evs[0].Kind != StateUpdate || evs[len(evs)-1].Kind != RecDue {
		t.Fatalf("state must come first and the reminder last: %+v", evs)
	}

	plain := func(name string, mod func(in *Input)) {
		t.Helper()
		in := adaptiveIn(at(9, 52, 0))
		mod(&in)
		got := breakEvents(Step(in))
		if len(got) != 1 || got[0].Kind != Notify || got[0].RecKind != "" || got[0].RecLengthMS != 0 {
			t.Errorf("%s: %+v, want one plain Notify", name, got)
		}
	}
	plain("notify off", func(in *Input) { in.Cfg.Notify = false })
	plain("disabled", func(in *Input) { in.Rec.Enabled = false })

	// A break the user took holds the reminder back until it ends.
	in := adaptiveIn(at(9, 52, 0))
	in.State.BreakTakenAt, in.State.BreakUntil = at(9, 45, 0), at(9, 55, 0)
	if evs := Step(in); len(breakEvents(evs)) != 0 || len(only(evs, StateUpdate)) != 0 {
		t.Fatalf("during the break: %+v, want nothing", evs)
	}

	// After it the run counts from the break's end although the screen stayed busy.
	in = adaptiveIn(at(10, 31, 0))
	in.State.BreakTakenAt, in.State.BreakUntil = at(9, 30, 0), at(9, 40, 0)
	if got := breakEvents(Step(in)); len(got) != 0 {
		t.Fatalf("10:31: %+v, want none", got)
	}
	in = adaptiveIn(at(10, 32, 0))
	in.State.BreakTakenAt, in.State.BreakUntil = at(9, 30, 0), at(9, 40, 0)
	got = breakEvents(Step(in))
	if len(got) != 1 || got[0].Body != "52 min without a break. Stand up for 10 min." {
		t.Fatalf("10:32: %+v", got)
	}
}

func TestBreakOver(t *testing.T) {
	taken := func(now int64, st State) []Event {
		in := input(now, vw{from: at(9, 0, 0), to: now, kind: "deep"})
		in.State = st
		return Step(in)
	}
	brk := State{BreakTakenAt: at(9, 53, 0), BreakUntil: at(10, 3, 0)}
	if evs := taken(at(10, 2, 0), brk); len(evs) != 0 {
		t.Fatalf("10:02: %+v, want nothing", evs)
	}
	evs := taken(at(10, 3, 0), brk)
	notes, states := only(evs, Notify), only(evs, StateUpdate)
	if len(notes) != 1 || notes[0].Topic != TopicBreakOver || notes[0].Title != "Break over" ||
		notes[0].Body != "10 min break done. Back to it." || notes[0].OfSession {
		t.Fatalf("10:03 notes = %+v", notes)
	}
	if len(states) != 1 || states[0].State.BreakOverAt != at(10, 3, 0) || states[0].State.BreakUntil != at(10, 3, 0) {
		t.Fatalf("10:03 state = %+v", states)
	}
	if evs[len(evs)-1].Kind != Notify || evs[0].Kind != StateUpdate {
		t.Fatalf("state first, notice last: %+v", evs)
	}

	done := brk
	done.BreakOverAt = at(10, 3, 0)
	if evs := taken(at(10, 3, 5), done); len(evs) != 0 {
		t.Fatalf("already sent: %+v, want nothing", evs)
	}

	// More than 5 min late: no notice, but never later either.
	evs = taken(at(10, 8, 1), brk)
	if n := only(evs, Notify); len(n) != 0 {
		t.Fatalf("late: %+v, want no notice", n)
	}
	if st := only(evs, StateUpdate); len(st) != 1 || st[0].State.BreakOverAt != at(10, 3, 0) {
		t.Fatalf("late state = %+v, want BreakOverAt set", st)
	}
	if n := only(taken(at(10, 7, 59), brk), Notify); len(n) != 1 {
		t.Fatalf("10:07:59: %+v, want the notice (under 5 min late)", n)
	}

	// No break taken: no Break over notice (the 63 min run has its own reminder).
	for _, e := range taken(at(10, 3, 0), State{}) {
		if e.Topic == TopicBreakOver {
			t.Fatalf("no break: %+v", e)
		}
	}

	// A call does not delay it.
	in := input(at(10, 3, 0), vw{from: at(9, 0, 0), to: at(10, 3, 0), kind: "deep"})
	in.State, in.Call = brk, true
	if n := only(Step(in), Notify); len(n) != 1 || n[0].Topic != TopicBreakOver {
		t.Fatalf("during a call: %+v", n)
	}
}

func TestRecommendDuringBreak(t *testing.T) {
	in := recBase()
	in.State.BreakUntil = at(9, 31, 0)
	if RecommendDue(in) || len(only(Step(in), RecDue)) != 0 {
		t.Fatal("a suggestion came during a taken break")
	}
	// RecommendDue only gates on the break itself; the quiet time after it
	// ends is stepRecommend's (TestRecommendWaitsForBreaks).
	in.State.BreakUntil = at(9, 30, 0)
	if !RecommendDue(in) || len(only(Step(in), RecDue)) != 0 {
		t.Fatal("a suggestion came the moment the break ended")
	}
	in.State.BreakUntil = at(9, 15, 0)
	if !RecommendDue(in) || len(only(Step(in), RecDue)) != 1 {
		t.Fatal("no suggestion 15 min after the break ended")
	}
}

// TestBreakMuted: a break hour always answered Not now gets no reminder in that
// hour; the rows must be read, else the reminder fails open (T-12-22).
func TestBreakMuted(t *testing.T) {
	const day = 24 * 60 * minute
	muted := func(in Input) Input {
		in.RecsOK = true
		for i := int64(1); i <= 3; i++ {
			in.Recs = append(in.Recs, recommend.Rec{ID: i, Kind: recommend.KindBreak, Hour: 9,
				AtMS: at(9, 52, 0) - i*day, Reply: recommend.ReplyDismiss})
		}
		return in
	}
	evs := Step(muted(adaptiveIn(at(9, 52, 0))))
	if len(breakEvents(evs)) != 0 || len(only(evs, StateUpdate)) != 0 {
		t.Fatalf("09:52 muted: %+v, want no reminder and no state change", evs)
	}
	// The hour turns: the reminder that is still due comes.
	evs = Step(muted(adaptiveIn(at(10, 0, 0))))
	if got := breakEvents(evs); len(got) != 1 || got[0].Body != "60 min without a break. Stand up for 10 min." {
		t.Fatalf("10:00: %+v, want the reminder", got)
	}
	if st := only(evs, StateUpdate); len(st) != 1 {
		t.Fatalf("10:00 state = %+v", st)
	}
	// Rows not read: fail open.
	in := muted(adaptiveIn(at(9, 52, 0)))
	in.RecsOK = false
	if got := breakEvents(Step(in)); len(got) != 1 {
		t.Fatalf("RecsOK false: %+v, want the reminder", got)
	}
	// A focus mute does not mute breaks.
	in = muted(adaptiveIn(at(9, 52, 0)))
	for i := range in.Recs {
		in.Recs[i].Kind = recommend.KindFocus
	}
	if got := breakEvents(Step(in)); len(got) != 1 {
		t.Fatalf("focus rows muted the break: %+v", got)
	}
	// Recommendations off: no learning, the old reminder.
	in = muted(adaptiveIn(at(9, 52, 0)))
	in.Rec.Enabled = false
	in.Profile = recommend.Profile{}
	in.Cfg.BreakAfterMS = 50 * minute
	if got := breakEvents(Step(in)); len(got) != 1 {
		t.Fatalf("disabled: %+v, want the reminder", got)
	}

	// RecsDue is true on a tick where a break is due, false the minute before.
	if RecsDue(adaptiveIn(at(9, 51, 0))) {
		t.Error("RecsDue at 09:51, nothing due")
	}
	if !RecsDue(adaptiveIn(at(9, 52, 0))) {
		t.Error("RecsDue at 09:52, break due")
	}
	in = adaptiveIn(at(9, 52, 0))
	in.Rec.Enabled = false
	if RecsDue(in) {
		t.Error("RecsDue with recommendations off")
	}
	in = adaptiveIn(at(9, 52, 0))
	in.Cfg.Notify = false
	if RecsDue(in) {
		t.Error("RecsDue with notify off")
	}
}
