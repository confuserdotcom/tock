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
