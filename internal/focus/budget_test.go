package focus

import (
	"testing"

	"tock/internal/classify"
)

// budgeted is a tick at now, 60 min into deep work, with a nudge budget of
// budget a day spaced gap apart.
func budgeted(now int64, budget int, gap int64, st State) Input {
	in := input(now, vw{now - 60*minute, now, classify.KindDeep, "", ""})
	in.Cfg.NoticeBudget, in.Cfg.NoticeGapMS = budget, gap
	in.State = st
	return in
}

func breakNotes(evs []Event) int {
	n := 0
	for _, e := range evs {
		if e.Topic == TopicBreak {
			n++
		}
	}
	return n
}

func savedState(t *testing.T, evs []Event) State {
	t.Helper()
	ss := only(evs, StateUpdate)
	if len(ss) != 1 {
		t.Fatalf("state updates = %d", len(ss))
	}
	return ss[0].State
}

// TestNudgeCountedAndSpaced: a nudge spends the day's budget and starts the
// gap; the next one waits the gap out.
func TestNudgeCountedAndSpaced(t *testing.T) {
	now := at(15, 0, 0)
	evs := Step(budgeted(now, 8, 20*minute, State{}))
	if breakNotes(evs) != 1 {
		t.Fatalf("no break reminder: %+v", evs)
	}
	st := savedState(t, evs)
	if st.NudgeDay != "2026-10-10" || st.Nudges != 1 || st.NudgeAt != now {
		t.Fatalf("budget state = %q %d %d", st.NudgeDay, st.Nudges, st.NudgeAt)
	}
	// 10 min later the break repeat is due, the gap is not over.
	later := budgeted(now+10*minute, 8, 20*minute, st)
	if evs := Step(later); breakNotes(evs) != 0 || len(only(evs, StateUpdate)) != 0 {
		t.Fatalf("a nudge inside the gap: %+v", evs)
	}
	if evs := Step(budgeted(now+20*minute, 8, 20*minute, st)); breakNotes(evs) != 1 {
		t.Fatalf("no nudge once the gap is over: %+v", evs)
	}
}

// TestNudgeBudgetSpent: once the day's budget is spent no nudge goes out;
// the next day starts afresh.
func TestNudgeBudgetSpent(t *testing.T) {
	now := at(15, 0, 0)
	st := State{NudgeDay: "2026-10-10", Nudges: 8, NudgeAt: now - 60*minute}
	if evs := Step(budgeted(now, 8, 20*minute, st)); breakNotes(evs) != 0 {
		t.Fatalf("a nudge past the budget: %+v", evs)
	}
	st.NudgeDay = "2026-10-09"
	if evs := Step(budgeted(now, 8, 20*minute, st)); breakNotes(evs) != 1 {
		t.Fatalf("yesterday's count held today back: %+v", evs)
	}
}

// TestNudgeOffKeepsState: with no budget and no gap nothing is counted.
func TestNudgeOffKeepsState(t *testing.T) {
	evs := Step(budgeted(at(15, 0, 0), 0, 0, State{}))
	if st := savedState(t, evs); st.NudgeDay != "" || st.Nudges != 0 || st.NudgeAt != 0 {
		t.Fatalf("counted with the budget off: %+v", st)
	}
}

// TestNudgeHeldInCall: a call holds a nudge back without advancing its
// state, so it comes after the call.
func TestNudgeHeldInCall(t *testing.T) {
	in := budgeted(at(15, 0, 0), 8, 20*minute, State{})
	in.Call = true
	in.Cfg.RecapAt = -1
	if evs := Step(in); len(evs) != 0 {
		t.Fatalf("events in a call: %+v", evs)
	}
}

// TestNudgeBudgetSparesAskedNotices: Break over is not a nudge: the
// budget never holds it back.
func TestNudgeBudgetSparesAskedNotices(t *testing.T) {
	now := at(15, 0, 0)
	st := State{NudgeDay: "2026-10-10", Nudges: 8, NudgeAt: now, BreakTakenAt: now - 6*minute, BreakUntil: now - minute}
	evs := Step(budgeted(now, 8, 20*minute, st))
	found := false
	for _, e := range evs {
		found = found || e.Topic == TopicBreakOver
	}
	if !found {
		t.Fatalf("Break over held back: %+v", evs)
	}
}
