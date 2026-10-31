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

// overInput is a tick at now over vs on a work day that ends at 18:00, with
// the routines on.
func overInput(now int64, vs ...vw) Input {
	in := startInput(now, vs...)
	in.Hours = stats.Hours{Set: true, StartMin: 9 * 60, EndMin: 18 * 60,
		Days: [7]bool{true, true, true, true, true, true, true}}
	return in
}

func TestOverDue(t *testing.T) {
	deep := func(h1, m1, h2, m2 int) vw { return vw{at(h1, m1, 0), at(h2, m2, 0), classify.KindDeep, "", ""} }
	idle := func(h1, m1, h2, m2 int) vw { return vw{at(h1, m1, 0), at(h2, m2, 0), "idle", "", ""} }
	for _, tt := range []struct {
		name string
		in   Input
		want bool
		over int64
	}{
		{"29 min", overInput(at(18, 29, 0), deep(17, 0, 18, 29)), false, 29 * minute},
		{"30 min", overInput(at(18, 30, 0), deep(17, 0, 18, 30)), true, 30 * minute},
		{"idle now", overInput(at(18, 45, 0), deep(17, 0, 18, 40), idle(18, 40, 18, 45)), false, 40 * minute},
		{"stopped, still idle", overInput(at(19, 0, 0), deep(17, 0, 18, 20), idle(18, 20, 19, 0)), false, 20 * minute},
		{"came back", overInput(at(19, 10, 0), deep(17, 0, 18, 20), idle(18, 20, 19, 0), deep(19, 0, 19, 10)), true, 30 * minute},
	} {
		if got := OverDue(tt.in); got != tt.want {
			t.Errorf("%s: OverDue = %v, want %v", tt.name, got, tt.want)
		}
		if got := OverMS(tt.in); got != tt.over {
			t.Errorf("%s: OverMS = %d, want %d", tt.name, got, tt.over)
		}
	}

	sent := func(now int64, day string, sentAt int64) Input {
		in := overInput(now, deep(17, 0, 23, 59))
		in.Views[0].End = now
		in.State = State{OverDay: day, OverAt: sentAt}
		return in
	}
	if OverDue(sent(at(19, 29, 0), "2026-10-10", at(18, 30, 0))) {
		t.Error("due before the repeat interval")
	}
	if !OverDue(sent(at(19, 30, 0), "2026-10-10", at(18, 30, 0))) {
		t.Error("not due after the repeat interval")
	}
	if !OverDue(sent(at(18, 30, 0), "2026-10-09", at(18, 30, 0)-24*60*minute)) {
		t.Error("yesterday's notice holds today's back")
	}

	work := deep(17, 0, 18, 30)
	unset := overInput(at(18, 30, 0), work)
	unset.Hours = stats.Hours{}
	if OverDue(unset) || OverMS(unset) != 0 {
		t.Error("due without a schedule")
	}
	dayOff := overInput(at(18, 30, 0), work)
	dayOff.Hours.Days = [7]bool{time.Monday: true}
	if OverDue(dayOff) || OverMS(dayOff) != 0 {
		t.Error("due on a day off (day0 is a Saturday)")
	}
	off := overInput(at(18, 30, 0), work)
	off.Routine.Overwork = false
	if OverDue(off) {
		t.Error("due when switched off")
	}
	call := overInput(at(18, 30, 0), work)
	call.Call = true
	if OverDue(call) {
		t.Error("due during a call")
	}
	call.Call = false
	if !OverDue(call) {
		t.Error("not due once the call is over")
	}
}

func TestStepOver(t *testing.T) {
	in := overInput(at(18, 30, 0), vw{at(17, 0, 0), at(18, 30, 0), classify.KindDeep, "", ""})
	in.Cfg.BreakAfterMS = 8 * 60 * minute
	in.OverBody = "O."
	evs := Step(in)
	notes := only(evs, Notify)
	if len(notes) != 1 || notes[0] != (Event{Kind: Notify, Topic: TopicOver, Title: TitleOver, Body: "O."}) {
		t.Fatalf("notes = %+v", notes)
	}
	if st := only(evs, StateUpdate); len(st) != 1 || st[0].State != (State{OverDay: "2026-10-10", OverAt: at(18, 30, 0)}) {
		t.Fatalf("state = %+v", st)
	}

	in.OverBody = ""
	evs = Step(in)
	if len(only(evs, Notify)) != 0 || len(only(evs, StateUpdate)) != 0 {
		t.Fatalf("no body, got %+v", evs)
	}
}

// TestStartDueCarryOver: work that began before local midnight and goes on
// is no start of day; a fresh run after an idle reset is. The app clips
// views to midnight, so a carried-over run starts at Day.
func TestStartDueCarryOver(t *testing.T) {
	deep := func(h1, m1, h2, m2 int) vw { return vw{at(h1, m1, 0), at(h2, m2, 0), classify.KindDeep, "", ""} }
	idle := func(h1, m1, h2, m2 int) vw { return vw{at(h1, m1, 0), at(h2, m2, 0), "idle", "", ""} }
	for _, tt := range []struct {
		name string
		in   Input
		want bool
	}{
		{"overnight run", startInput(at(0, 40, 0), deep(0, 0, 0, 40)), false},
		{"overnight run, short pause", startInput(at(1, 0, 0), deep(0, 0, 0, 30), idle(0, 30, 0, 33), deep(0, 33, 1, 0)), false},
		{"overnight run, idle now", startInput(at(1, 30, 0), deep(0, 0, 0, 30), idle(0, 30, 1, 30)), false},
		{"fresh run after idle reset", startInput(at(1, 35, 0), deep(0, 0, 0, 30), idle(0, 30, 1, 30), deep(1, 30, 1, 35)), true},
		{"fresh run after a gap", startInput(at(8, 5, 0), deep(0, 0, 0, 30), deep(8, 0, 8, 5)), true},
	} {
		if got := StartDue(tt.in); got != tt.want {
			t.Errorf("%s: StartDue = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestOverDueMidnightRollover pins that the overworking notice goes quiet at
// local midnight: the new day's work end has not come.
func TestOverDueMidnightRollover(t *testing.T) {
	in := overInput(at(0, 40, 0), vw{at(0, 0, 0), at(0, 40, 0), classify.KindDeep, "", ""})
	if OverDue(in) || OverMS(in) != 0 {
		t.Error("overwork notice after midnight, before the new day's work end")
	}
}
