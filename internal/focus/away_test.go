package focus

import (
	"testing"

	"tock/internal/classify"
)

func TestStepAway(t *testing.T) {
	abs := &classify.Interval{From: at(12, 10, 0), To: at(12, 45, 0)}
	in := input(at(12, 46, 0), vw{from: at(12, 0, 0), to: at(12, 10, 0)}, vw{from: at(12, 45, 0), to: at(12, 46, 0)})
	in.Absence = abs
	evs := Step(in)
	notes := only(evs, Notify)
	if len(notes) != 1 || notes[0].Title != TitleAway ||
		notes[0].Body != "Away 35m (12:10–12:45). What was it? Run tock away or open tock." {
		t.Fatalf("notes = %+v", notes)
	}
	st := only(evs, StateUpdate)
	if len(st) != 1 || st[0].State.AwayAt != abs.To {
		t.Fatalf("state = %+v", st)
	}

	in.State = st[0].State
	if notes := only(Step(in), Notify); len(notes) != 0 {
		t.Fatalf("asked twice: %+v", notes)
	}

	late := input(abs.To+AwayNotifyMS+1, vw{from: at(12, 0, 0), to: at(12, 10, 0)}, vw{from: at(12, 45, 0), to: abs.To + AwayNotifyMS + 1})
	late.Absence = abs
	if notes := only(Step(late), Notify); len(notes) != 0 {
		t.Fatalf("asked after the window: %+v", notes)
	}

	none := input(at(12, 46, 0), vw{from: at(12, 0, 0), to: at(12, 46, 0)})
	if evs := Step(none); len(only(evs, Notify)) != 0 || len(only(evs, StateUpdate)) != 0 {
		t.Fatalf("no absence: %+v", evs)
	}
}
