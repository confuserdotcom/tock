package focus

import (
	"reflect"
	"testing"

	"tock/internal/classify"
)

func TestAutoSessions(t *testing.T) {
	deep, dis := classify.KindDeep, classify.KindDistract
	ended := func() Input {
		return input(at(9, 35, 0), vw{at(9, 0, 0), at(9, 30, 0), deep, "", ""}, vw{at(9, 30, 0), at(9, 35, 0), dis, "", ""})
	}
	got := only(Step(ended()), AutoSession)
	want := []Event{{Kind: AutoSession, Sess: Session{Kind: KindAuto, StartMS: at(9, 0, 0), EndMS: at(9, 30, 0),
		DeepMS: 30 * minute, LongestMS: 30 * minute}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ended block = %+v, want %+v", got, want)
	}

	short := input(at(9, 35, 0), vw{at(9, 0, 0), at(9, 24, 0), deep, "", ""}, vw{at(9, 24, 0), at(9, 35, 0), dis, "", ""})
	if got := only(Step(short), AutoSession); len(got) != 0 {
		t.Fatalf("24 min block = %+v", got)
	}

	open := input(at(9, 30, 30), vw{at(9, 0, 0), at(9, 30, 0), deep, "", ""}, vw{at(9, 30, 0), at(9, 30, 30), dis, "", ""})
	if got := only(Step(open), AutoSession); len(got) != 0 {
		t.Fatalf("block may still continue = %+v", got)
	}

	recorded := ended()
	recorded.Sessions = []Session{{ID: 3, Kind: KindAuto, StartMS: at(9, 0, 0), EndMS: at(9, 30, 0)}}
	if got := only(Step(recorded), AutoSession); len(got) != 0 {
		t.Fatalf("already recorded = %+v", got)
	}

	manual := ended()
	manual.Sessions = []Session{{ID: 4, Kind: KindManual, StartMS: at(9, 10, 0), PlannedMS: at(9, 20, 0), EndMS: at(9, 20, 0)}}
	if got := only(Step(manual), AutoSession); len(got) != 0 {
		t.Fatalf("overlaps a manual session = %+v", got)
	}

	runningManual := ended()
	runningManual.Sessions = []Session{{ID: 5, Kind: KindManual, StartMS: at(9, 20, 0), PlannedMS: at(9, 45, 0)}}
	if got := only(Step(runningManual), AutoSession); len(got) != 0 {
		t.Fatalf("overlaps the running session = %+v", got)
	}
}

func TestRecap(t *testing.T) {
	base := func(now int64) Input {
		in := input(now, vw{at(9, 0, 0), at(10, 0, 0), classify.KindDeep, "", ""})
		in.RecapBody = "R."
		return in
	}
	in := base(at(21, 0, 0))
	if !RecapDue(in) {
		t.Fatal("recap not due at 21:00")
	}
	evs := Step(in)
	notes := only(evs, Notify)
	if len(notes) != 1 || notes[0] != (Event{Kind: Notify, Topic: TopicRecap, Title: TitleRecap, Body: "R."}) {
		t.Fatalf("notes = %+v", notes)
	}
	if st := only(evs, StateUpdate); len(st) != 1 || st[0].State != (State{RecapDay: "2026-10-10"}) {
		t.Fatalf("state = %+v", st)
	}

	if RecapDue(base(at(20, 59, 0))) {
		t.Error("due before 21:00")
	}
	if !RecapDue(base(at(23, 30, 0))) {
		t.Error("not due after a late start")
	}
	sent := base(at(21, 5, 0))
	sent.State.RecapDay = "2026-10-10"
	if RecapDue(sent) {
		t.Error("due twice in a day")
	}
	off := base(at(21, 0, 0))
	off.Cfg.RecapAt = -1
	if RecapDue(off) {
		t.Error("due when off")
	}
	quiet := input(at(21, 0, 0), vw{at(9, 0, 0), at(10, 0, 0), "idle", "", ""})
	if RecapDue(quiet) {
		t.Error("due on a day with no activity")
	}
	noBody := base(at(21, 0, 0))
	noBody.RecapBody = ""
	if evs := Step(noBody); len(only(evs, Notify)) != 0 || len(only(evs, StateUpdate)) != 0 {
		t.Errorf("recap without a body: %+v", evs)
	}
}
