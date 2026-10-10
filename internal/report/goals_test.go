package report

import (
	"testing"

	"tock/internal/classify"
)

var (
	studyDay  = classify.Goal{Index: 1, Category: "Study", MS: 2 * 3_600_000}
	tockWeek  = classify.Goal{Index: 2, Project: "tock", MS: 10 * 3_600_000, Week: true}
	funLimit  = classify.Goal{Index: 3, Category: "Entertainment", AtMost: true, MS: 45 * 60_000}
	testGoals = []classify.Goal{studyDay, tockWeek, funLimit}
)

// onDay moves views by n days.
func onDay(vs []classify.SpanView, n int) []classify.SpanView {
	out := append([]classify.SpanView{}, vs...)
	for i := range out {
		out[i].Start += int64(n) * 86_400_000
		out[i].End += int64(n) * 86_400_000
	}
	return out
}

func TestGoalsStates(t *testing.T) {
	monday := MondayOf(day0)
	// Thursday: 3h of tock; Saturday (day0): 1h study, 1h tock, 50m fun.
	thu, thuL := build(t, sv{id: 1, start: "09:00", end: "12:00", app: "term", cat: "Code", kind: "deep", proj: "tock"})
	sat, satL := build(t,
		sv{id: 2, start: "09:00", end: "10:00", app: "pdf", cat: "Study", kind: "deep"},
		sv{id: 3, start: "10:00", end: "10:30", kind: "idle"},
		sv{id: 4, start: "10:30", end: "11:30", app: "term", cat: "Code", kind: "deep", proj: "tock"},
		sv{id: 5, start: "11:30", end: "12:20", app: "yt", cat: "Entertainment", kind: "distract"},
	)
	vs := append(onDay(thu, -2), sat...)
	ls := append(thuL, satL...)
	rows := Goals(testGoals, vs, ls, day0, monday)
	want := []struct {
		done  int64
		state string
	}{{3_600_000, GoalShort}, {4 * 3_600_000, GoalShort}, {50 * 60_000, GoalOver}}
	for i, w := range want {
		if rows[i].DoneMS != w.done || rows[i].State != w.state {
			t.Errorf("goal %d = %+v, want %d %s", i+1, rows[i], w.done, w.state)
		}
	}
	if got := GoalsLine(rows); got != "Study 1h00/2h00 · project tock 4h00/10h00 wk · Entertainment 50m/45m over" {
		t.Fatalf("line = %q", got)
	}
	if held, over := GoalsHeld(rows); held != 0 || over != "Entertainment" {
		t.Fatalf("held %d, over %q", held, over)
	}

	// Thursday alone: the week goal only counts up to Thursday's end.
	rows = Goals(testGoals, vs, ls, day0.AddDate(0, 0, -2), monday)
	if rows[1].DoneMS != 3*3_600_000 || rows[2].State != GoalOK || rows[0].DoneMS != 0 {
		t.Fatalf("thursday = %+v", rows)
	}
	if held, over := GoalsHeld(rows); held != 1 || over != "" {
		t.Fatalf("held %d, over %q", held, over)
	}
}

func TestWeekGoals(t *testing.T) {
	monday := MondayOf(day0)
	a, al := build(t,
		sv{id: 1, start: "09:00", end: "11:30", app: "pdf", cat: "Study", kind: "deep"},
		sv{id: 2, start: "12:00", end: "13:00", app: "yt", cat: "Entertainment", kind: "distract"},
	)
	b, bl := build(t, sv{id: 3, start: "09:00", end: "10:00", app: "pdf", cat: "Study", kind: "deep"})
	idleOnly, il := build(t, sv{id: 4, start: "09:00", end: "10:00", kind: "idle"})
	// Monday: study met, fun over. Wednesday: study short. Thursday: idle only.
	// Saturday (day0): 6h of tock. Sunday: future.
	tockDay, tl := build(t, sv{id: 5, start: "08:00", end: "14:00", app: "term", cat: "Code", kind: "deep", proj: "tock"})
	future, fl := build(t, sv{id: 6, start: "08:00", end: "09:00", app: "pdf", cat: "Study", kind: "deep"})
	var vs []classify.SpanView
	var ls []classify.Label
	for _, p := range []struct {
		vs  []classify.SpanView
		ls  []classify.Label
		day int
	}{{a, al, -5}, {b, bl, -3}, {idleOnly, il, -2}, {tockDay, tl, 0}, {future, fl, 1}} {
		vs = append(vs, onDay(p.vs, p.day)...)
		ls = append(ls, p.ls...)
	}
	now := hm(t, "15:00")
	rows := WeekGoals(testGoals, vs, ls, monday, now)
	// Active days: Mon, Wed, Sat (Thu is idle only, Sun is future).
	if r := rows[0]; r.Days != 3 || r.Held != 1 || WeekGoalText(r) != "met 1 of 3 days" {
		t.Errorf("study = %+v %q", r, WeekGoalText(r))
	}
	if r := rows[1]; r.DoneMS != 6*3_600_000 || WeekGoalText(r) != "6h00/10h00 short 4h00" {
		t.Errorf("tock = %+v %q", r, WeekGoalText(r))
	}
	if r := rows[2]; r.Days != 3 || r.Held != 2 || WeekGoalText(r) != "over 1 of 3 days" {
		t.Errorf("fun = %+v %q", r, WeekGoalText(r))
	}
	if got := WeekGoalText(WeekGoalRow{Goal: tockWeek, DoneMS: 12 * 3_600_000, State: GoalMet}); got != "12h00/10h00 met" {
		t.Errorf("met week = %q", got)
	}
	if got := WeekGoalText(WeekGoalRow{Goal: studyDay}); got != "no active days" {
		t.Errorf("empty = %q", got)
	}
}

func TestStatusGoals(t *testing.T) {
	in := StatusInput{TrackedMS: 3_600_000, TrackdUp: true, Goals: []GoalRow{
		{Goal: studyDay, State: GoalMet}, {Goal: tockWeek, State: GoalShort}, {Goal: funLimit, State: GoalOver}}}
	if got := Status(in); got != "1h00 · goals 1/3 · over Entertainment" {
		t.Fatalf("status = %q", got)
	}
	in.Goals[2].State = GoalOK
	if got := Status(in); got != "1h00 · goals 2/3" {
		t.Fatalf("status = %q", got)
	}
}
