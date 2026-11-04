package focus

import (
	"testing"

	"tock/internal/classify"
)

// screenIn is a tick at now after a break taken at 14:00 until 14:10, with
// the break screen on (5 min grace) and the views vs.
func screenIn(now int64, st State, vs ...vw) Input {
	in := input(now, vs...)
	in.Cfg.BreakScreen, in.Cfg.BreakScreenGraceMS = true, 5*minute
	st.BreakTakenAt, st.BreakUntil = at(14, 0, 0), at(14, 10, 0)
	st.BreakOverAt = at(14, 10, 0) // Break over went out
	in.State = st
	return in
}

func screens(evs []Event) []Event {
	var out []Event
	for _, e := range evs {
		if e.Kind == Notify && e.Topic == TopicBreakScreen {
			out = append(out, e)
		}
	}
	return out
}

func TestBreakScreen(t *testing.T) {
	away := vw{at(13, 0, 0), at(14, 0, 0), classify.KindDeep, "", ""}
	idleNow := func(now int64) vw { return vw{at(14, 0, 0), now, "idle", "", ""} }
	for _, c := range []struct {
		name   string
		in     Input
		screen bool
		mark   bool // BreakScreenAt is set to the break's end
	}{
		{"inside the grace", screenIn(at(14, 14, 0), State{}, away, idleNow(at(14, 14, 0))), false, false},
		{"still away after the grace", screenIn(at(14, 15, 0), State{}, away, idleNow(at(14, 15, 0))), true, true},
		{"on a distraction", screenIn(at(14, 15, 0), State{}, away,
			vw{at(14, 0, 0), at(14, 15, 0), classify.KindDistract, "", ""}), true, true},
		{"back at deep work", screenIn(at(14, 15, 0), State{}, away,
			vw{at(14, 0, 0), at(14, 15, 0), classify.KindDeep, "", ""}), false, true},
		{"already shown", screenIn(at(14, 16, 0), State{BreakScreenAt: at(14, 10, 0)}, away, idleNow(at(14, 16, 0))), false, false},
		{"a late tick gives up", screenIn(at(14, 20, 0), State{}, away, idleNow(at(14, 20, 0))), false, true},
	} {
		evs := Step(c.in)
		if got := len(screens(evs)) == 1; got != c.screen {
			t.Errorf("%s: screen = %v, want %v: %+v", c.name, got, c.screen, evs)
		}
		marked := false
		for _, e := range only(evs, StateUpdate) {
			marked = e.State.BreakScreenAt == at(14, 10, 0)
		}
		if marked != c.mark {
			t.Errorf("%s: marked = %v, want %v", c.name, marked, c.mark)
		}
	}
}

func TestBreakScreenEvent(t *testing.T) {
	in := screenIn(at(14, 15, 0), State{}, vw{at(14, 0, 0), at(14, 15, 0), "idle", "", ""})
	s := screens(Step(in))
	if len(s) != 1 {
		t.Fatal("no screen")
	}
	if s[0].Title != TitleBreakScreen || s[0].Body != "Your 10 min break ran 5 min over." ||
		s[0].Break != (classify.Interval{From: at(14, 0, 0), To: at(14, 10, 0)}) {
		t.Fatalf("screen = %+v", s[0])
	}
}

func TestBreakScreenOffOrInCall(t *testing.T) {
	in := screenIn(at(14, 15, 0), State{}, vw{at(14, 0, 0), at(14, 15, 0), "idle", "", ""})
	in.Cfg.BreakScreen = false
	if s := screens(Step(in)); len(s) != 0 {
		t.Fatalf("screen while off: %+v", s)
	}
	in.Cfg.BreakScreen, in.Call = true, true
	evs := Step(in)
	if s := screens(evs); len(s) != 0 {
		t.Fatalf("screen in a call: %+v", s)
	}
	for _, e := range only(evs, StateUpdate) {
		if e.State.BreakScreenAt != 0 {
			t.Fatal("a call marked the screen shown: it must wait")
		}
	}
}
