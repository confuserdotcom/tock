package focus

import (
	"testing"
	"time"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/stats"
)

// startInput is a tick at now over vs with the routines on.
func startInput(now int64, vs ...vw) Input {
	in := input(now, vs...)
	in.Routine = config.DefaultRoutine()
	return in
}

func TestStartDue(t *testing.T) {
	work := vw{at(9, 0, 0), at(9, 30, 0), classify.KindDeep, "", ""}
	if !StartDue(startInput(at(9, 30, 0), work)) {
		t.Fatal("not due after 30 min of activity")
	}
	if StartDue(startInput(at(9, 0, 30), vw{at(9, 0, 0), at(9, 0, 30), classify.KindDeep, "", ""})) {
		t.Error("due under 1 min of activity")
	}
	sent := startInput(at(9, 30, 0), work)
	sent.State.StartDay = "2026-10-10"
	if StartDue(sent) {
		t.Error("due twice in a day")
	}
	off := startInput(at(9, 30, 0), work)
	off.Routine.Start = false
	if StartDue(off) {
		t.Error("due when switched off")
	}
	weekdays := startInput(at(9, 30, 0), work)
	weekdays.Hours = stats.Hours{Set: true, StartMin: 9 * 60, EndMin: 18 * 60,
		Days: [7]bool{time.Monday: true, time.Tuesday: true, time.Wednesday: true, time.Thursday: true, time.Friday: true}}
	if StartDue(weekdays) {
		t.Error("due on a day off (day0 is a Saturday)")
	}
	call := startInput(at(9, 30, 0), work)
	call.Call = true
	if StartDue(call) {
		t.Error("due during a call")
	}
	call.Call = false
	if !StartDue(call) {
		t.Error("not due once the call is over")
	}
	idle := startInput(at(9, 30, 0), vw{at(9, 0, 0), at(9, 30, 0), "idle", "", ""})
	if StartDue(idle) {
		t.Error("due on idle time only")
	}
	if !StartDue(startInput(at(23, 0, 0), vw{at(22, 58, 0), at(23, 0, 0), classify.KindDeep, "", ""})) {
		t.Error("a late first activity does not bring the notice")
	}
}

func TestStepStart(t *testing.T) {
	in := startInput(at(9, 30, 0), vw{at(9, 0, 0), at(9, 30, 0), classify.KindDeep, "", ""})
	in.StartBody = "S."
	evs := Step(in)
	notes := only(evs, Notify)
	if len(notes) != 1 || notes[0] != (Event{Kind: Notify, Topic: TopicStart, Title: TitleStart, Body: "S."}) {
		t.Fatalf("notes = %+v", notes)
	}
	if st := only(evs, StateUpdate); len(st) != 1 || st[0].State != (State{StartDay: "2026-10-10"}) {
		t.Fatalf("state = %+v", st)
	}

	in.StartBody = ""
	evs = Step(in)
	if len(only(evs, Notify)) != 0 || len(only(evs, StateUpdate)) != 0 {
		t.Fatalf("no body, got %+v", evs)
	}
}

// middayInput is a tick at now over vs with the routines on.
func middayInput(now int64, vs ...vw) Input { return startInput(now, vs...) }

func allDays(startMin, endMin int) stats.Hours {
	h := stats.Hours{Set: true, StartMin: startMin, EndMin: endMin}
	for i := range h.Days {
		h.Days[i] = true
	}
	return h
}

func TestMiddayDue(t *testing.T) {
	morning := vw{at(12, 0, 0), at(13, 0, 0), classify.KindDeep, "", ""}
	if !MiddayDue(middayInput(at(13, 0, 0), morning)) {
		t.Fatal("not due at 13:00")
	}
	if MiddayDue(middayInput(at(12, 59, 0), vw{at(12, 0, 0), at(12, 59, 0), classify.KindDeep, "", ""})) {
		t.Error("due at 12:59")
	}
	if !MiddayDue(middayInput(at(15, 0, 0), vw{at(14, 0, 0), at(15, 0, 0), classify.KindDeep, "", ""})) {
		t.Error("late start is not due")
	}
	late := middayInput(at(15, 0, 0), vw{at(14, 0, 0), at(15, 0, 0), classify.KindDeep, "", ""})
	late.Hours = allDays(9*60, 14*60)
	if MiddayDue(late) {
		t.Error("due after the work end")
	}
	before := middayInput(at(13, 30, 0), vw{at(12, 30, 0), at(13, 30, 0), classify.KindDeep, "", ""})
	before.Hours = allDays(9*60, 14*60)
	if !MiddayDue(before) {
		t.Error("not due before the work end")
	}
	off := middayInput(at(13, 0, 0), morning)
	off.Routine.MiddayAt = -1
	if MiddayDue(off) {
		t.Error("due when switched off")
	}
	sent := middayInput(at(13, 0, 0), morning)
	sent.State.MiddayDay = "2026-10-10"
	if MiddayDue(sent) {
		t.Error("due twice in a day")
	}
	dayOff := middayInput(at(13, 0, 0), morning)
	dayOff.Hours = stats.Hours{Set: true, StartMin: 9 * 60, EndMin: 18 * 60,
		Days: [7]bool{time.Monday: true, time.Tuesday: true, time.Wednesday: true, time.Thursday: true, time.Friday: true}}
	if MiddayDue(dayOff) {
		t.Error("due on a day off (day0 is a Saturday)")
	}
	call := middayInput(at(13, 0, 0), morning)
	call.Call = true
	if MiddayDue(call) {
		t.Error("due during a call")
	}
	call.Call = false
	if !MiddayDue(call) {
		t.Error("not due once the call is over")
	}
	if MiddayDue(middayInput(at(13, 0, 0), vw{at(12, 59, 30), at(13, 0, 0), classify.KindDeep, "", ""})) {
		t.Error("due on 30 s of activity")
	}
}

func TestStepMidday(t *testing.T) {
	in := middayInput(at(13, 0, 0), vw{at(12, 30, 0), at(13, 0, 0), classify.KindDeep, "", ""})
	in.MiddayBody = "M."
	evs := Step(in)
	notes := only(evs, Notify)
	if len(notes) != 1 || notes[0] != (Event{Kind: Notify, Topic: TopicMidday, Title: TitleMidday, Body: "M."}) {
		t.Fatalf("notes = %+v", notes)
	}
	if st := only(evs, StateUpdate); len(st) != 1 || st[0].State != (State{MiddayDay: "2026-10-10"}) {
		t.Fatalf("state = %+v", st)
	}
	in.MiddayBody = ""
	if evs := Step(in); len(only(evs, Notify)) != 0 || len(only(evs, StateUpdate)) != 0 {
		t.Fatalf("no body, got %+v", evs)
	}
}

func TestStepStartAndMidday(t *testing.T) {
	in := middayInput(at(13, 32, 0), vw{at(13, 30, 0), at(13, 32, 0), classify.KindDeep, "", ""})
	in.StartBody, in.MiddayBody = "S.", "M."
	evs := Step(in)
	notes := only(evs, Notify)
	if len(notes) != 2 || notes[0].Topic != TopicStart || notes[1].Topic != TopicMidday {
		t.Fatalf("notes = %+v", notes)
	}
	if st := only(evs, StateUpdate); len(st) != 1 || st[0].State != (State{StartDay: "2026-10-10", MiddayDay: "2026-10-10"}) {
		t.Fatalf("state = %+v", st)
	}
}
