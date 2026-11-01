package focus

import (
	"testing"

	"tock/internal/config"
	"tock/internal/recommend"
	"tock/internal/stats"
)

// workHours is every weekday 08:00-18:00.
func workHours() stats.Hours {
	h := stats.Hours{Set: true, StartMin: 8 * 60, EndMin: 18 * 60}
	for i := range h.Days {
		h.Days[i] = true
	}
	return h
}

// recInput is a Saturday 09:30 tick in the middle of the best focus hour 9.
func recBase() Input {
	in := input(at(9, 30, 0), vw{from: at(9, 0, 0), to: at(9, 30, 0), kind: "deep"})
	in.Rec = config.DefaultRecommend()
	in.Hours = workHours()
	in.Profile.Top[6] = []int{9}
	in.Profile.SessionMS = 45 * minute
	in.RecsOK = true
	return in
}

func TestStepRecommend(t *testing.T) {
	in := recBase()
	evs := only(Step(in), RecDue)
	if len(evs) != 1 {
		t.Fatalf("RecDue events = %+v, want 1", evs)
	}
	e := evs[0]
	if e.Topic != TopicRecommend || e.Title != "Good time to focus" ||
		e.Body != "Your best focus hour · start a 45 min session now?" ||
		e.Minutes != 45 || e.RecKind != recommend.KindFocus || e.RecLengthMS != 45*minute {
		t.Fatalf("event = %+v", e)
	}
	if !RecommendDue(in) || !RecsDue(in) {
		t.Error("RecommendDue / RecsDue false on the base input")
	}

	none := func(name string, mod func(in *Input), due bool) {
		t.Helper()
		in := recBase()
		mod(&in)
		if got := only(Step(in), RecDue); len(got) != 0 {
			t.Errorf("%s: got %+v, want none", name, got)
		}
		if RecommendDue(in) != due {
			t.Errorf("%s: RecommendDue = %v, want %v", name, !due, due)
		}
	}
	none("later hour", func(in *Input) {
		in.Now = at(10, 30, 0)
		in.Views, in.Labels = world(vw{from: at(10, 0, 0), to: at(10, 30, 0), kind: "deep"})
	}, false)
	none("disabled", func(in *Input) { in.Rec.Enabled = false }, false)
	none("max 0", func(in *Input) { in.Rec.MaxPerDay = 0 }, false)
	none("notify off", func(in *Input) { in.Cfg.Notify = false }, false)
	none("hours unset", func(in *Input) { in.Hours = stats.Hours{} }, false)
	none("before the window", func(in *Input) {
		in.Now = at(7, 30, 0)
		in.Profile.Top[6] = []int{7}
		in.Views, in.Labels = world(vw{from: at(7, 0, 0), to: at(7, 30, 0), kind: "deep"})
	}, false)
	none("day off", func(in *Input) { in.Hours.Days[6] = false }, false)
	none("manual session", func(in *Input) {
		in.Sessions = []Session{{ID: 1, Kind: KindManual, StartMS: at(9, 20, 0), PlannedMS: at(9, 45, 0)}}
	}, false)
	none("zero profile", func(in *Input) { in.Profile = recommend.Profile{} }, false)
	none("rows not read", func(in *Input) { in.RecsOK = false }, true)

	row := func(kind string, h, m int) recommend.Rec {
		return recommend.Rec{Kind: kind, AtMS: at(h, m, 0)}
	}
	none("two rows today", func(in *Input) {
		in.Recs = []recommend.Rec{row("focus", 8, 0), row("focus", 8, 30)}
	}, true)
	none("row 30 min ago", func(in *Input) { in.Recs = []recommend.Rec{row("focus", 9, 0)} }, true)
	in = recBase()
	in.Recs = []recommend.Rec{row("focus", 8, 15), row("break", 9, 20)}
	if len(only(Step(in), RecDue)) != 1 {
		t.Error("one focus row at 08:15 and a break row at 09:20: want the suggestion")
	}
	// A row from yesterday does not count against today.
	in = recBase()
	in.Recs = []recommend.Rec{{Kind: "focus", AtMS: at(9, 0, 0) - 24*60*minute}, {Kind: "focus", AtMS: at(9, 10, 0) - 24*60*minute}}
	if len(only(Step(in), RecDue)) != 1 {
		t.Error("rows of yesterday blocked today's suggestion")
	}
}

func TestRecommendRules(t *testing.T) {
	mins := func(in Input) []int {
		var out []int
		for _, e := range only(Step(in), RecDue) {
			out = append(out, e.Minutes)
		}
		return out
	}
	none := func(name string, mod func(in *Input)) {
		t.Helper()
		in := recBase()
		mod(&in)
		if got := mins(in); len(got) != 0 {
			t.Errorf("%s: suggested %v, want none", name, got)
		}
	}
	setViews := func(in *Input, vs ...vw) { in.Views, in.Labels = world(vs...) }
	none("call", func(in *Input) { in.Call = true })
	none("meeting view in front", func(in *Input) {
		setViews(in, vw{from: at(9, 0, 0), to: at(9, 29, 0), kind: "deep"}, vw{from: at(9, 29, 0), to: at(9, 30, 0), bundle: "us.zoom.xos"})
	})
	none("front view idle", func(in *Input) {
		setViews(in, vw{from: at(9, 0, 0), to: at(9, 25, 0), kind: "deep"}, vw{from: at(9, 25, 0), to: at(9, 30, 0), kind: "idle"})
	})
	none("under 5 min of own activity", func(in *Input) {
		setViews(in, vw{from: at(9, 0, 0), to: at(9, 26, 0), kind: "idle"}, vw{from: at(9, 26, 0), to: at(9, 30, 0), kind: "deep"})
	})
	none("another machine's views", func(in *Input) {
		for i := range in.Views {
			in.Views[i].ID = -in.Views[i].ID
		}
	})
	none("tock.* bundle in front", func(in *Input) {
		setViews(in, vw{from: at(9, 0, 0), to: at(9, 30, 0), kind: "deep", bundle: "tock.remote"})
	})
	none("distraction snooze", func(in *Input) { in.State.DistractSnoozeUntil = at(9, 40, 0) })
	none("running slot", func(in *Input) {
		in.RecSlots = []Slot{{Key: "k", StartMS: at(9, 0, 0), EndMS: at(10, 0, 0)}}
	})
	none("slot in 13 min", func(in *Input) {
		in.RecSlots = []Slot{{Key: "k", StartMS: at(9, 43, 0), EndMS: at(10, 0, 0)}}
	})
	none("work window ends in 12 min", func(in *Input) { in.Hours.EndMin = 9*60 + 42 })

	// With room: the length is capped by the next slot and the [hours] end,
	// floored to 5 min.
	for _, c := range []struct {
		name string
		mod  func(in *Input)
		want int
	}{
		{"slot in 25 min", func(in *Input) { in.RecSlots = []Slot{{Key: "k", StartMS: at(9, 55, 0), EndMS: at(10, 30, 0)}} }, 25},
		{"slot in 42 min", func(in *Input) { in.RecSlots = []Slot{{Key: "k", StartMS: at(10, 12, 0), EndMS: at(11, 0, 0)}} }, 40},
		{"window ends in 30 min", func(in *Input) { in.Hours.EndMin = 10 * 60 }, 30},
		{"slot ended already", func(in *Input) { in.RecSlots = []Slot{{Key: "k", StartMS: at(8, 50, 0), EndMS: at(9, 20, 0)}} }, 45},
		{"nearest of two slots", func(in *Input) {
			in.RecSlots = []Slot{{Key: "a", StartMS: at(10, 20, 0), EndMS: at(10, 40, 0)}, {Key: "b", StartMS: at(9, 55, 0), EndMS: at(10, 10, 0)}}
		}, 25},
	} {
		in := recBase()
		c.mod(&in)
		if got := mins(in); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: got %v, want [%d]", c.name, got, c.want)
		}
		if evs := only(Step(in), RecDue); len(evs) == 1 && evs[0].RecLengthMS != int64(c.want)*minute {
			t.Errorf("%s: RecLengthMS = %d", c.name, evs[0].RecLengthMS)
		}
	}
}

// TestRecommendLearning: replies to earlier suggestions mute an hour or make
// it need the first rank (REC-04).
func TestRecommendLearning(t *testing.T) {
	const day = 24 * 60 * minute
	rows := func(ageDays int64, replies ...string) []recommend.Rec {
		var out []recommend.Rec
		for i, r := range replies {
			out = append(out, recommend.Rec{ID: int64(i + 1), Kind: recommend.KindFocus, Hour: 9,
				AtMS: at(9, 10, 0) - (ageDays+int64(i))*day, Reply: r})
		}
		return out
	}
	dis, acc := recommend.ReplyDismiss, recommend.ReplyAccept
	suggests := func(top []int, recs []recommend.Rec) bool {
		in := recBase()
		in.Profile.Top[6] = top
		in.Recs = recs
		return len(only(Step(in), RecDue)) == 1
	}
	if suggests([]int{9}, rows(1, dis, dis, dis)) {
		t.Error("three dismisses at the hour: want no suggestion")
	}
	if !suggests([]int{9}, rows(1, dis, dis)) {
		t.Error("two dismisses: want the suggestion")
	}
	if suggests([]int{10, 9}, rows(1, dis, dis, dis, dis, acc)) {
		t.Error("1 of 5 accepted and rank 2: want no suggestion")
	}
	if !suggests([]int{9, 10}, rows(1, dis, dis, dis, dis, acc)) {
		t.Error("1 of 5 accepted and rank 1: want the suggestion")
	}
	if !suggests([]int{10, 9}, rows(1, dis, dis, dis, acc)) {
		t.Error("1 of 4 accepted (25 %) and rank 2: want the suggestion")
	}
	if !suggests([]int{9}, rows(31, dis, dis, dis)) {
		t.Error("replies 31 days old: want the suggestion (aged out)")
	}
	// A muted hour only mutes the focus kind of that hour.
	other := rows(1, dis, dis, dis)
	for i := range other {
		other[i].Hour = 10
	}
	if !suggests([]int{9}, other) {
		t.Error("replies of another hour muted this one")
	}
	// The rows are not read: no learning, no suggestion at all.
	in := recBase()
	in.RecsOK, in.Recs = false, rows(1, dis, dis, dis)
	if len(only(Step(in), RecDue)) != 0 {
		t.Error("suggested without rows read")
	}
}

// TestRecommendWaitsForBreaks: a focus suggestion does not come in the tick of
// a break reminder, nor shortly after a break reminder, the end-of-session
// break prompt or "Break over".
func TestRecommendWaitsForBreaks(t *testing.T) {
	kinds := func(in Input) (focus, brk int) {
		for _, e := range only(Step(in), RecDue) {
			switch e.RecKind {
			case recommend.KindFocus:
				focus++
			case recommend.KindBreak:
				brk++
			}
		}
		return focus, brk
	}
	// A 60 min run up to 09:30 is past the 50 min break_after in the best hour.
	long := recBase()
	long.Views, long.Labels = world(vw{from: at(8, 30, 0), to: at(9, 30, 0), kind: "deep"})
	if f, b := kinds(long); f != 0 || b != 1 {
		t.Errorf("break reminder tick: %d focus, %d break suggestions; want 0 and 1", f, b)
	}

	// The reminder was sent 5 min ago and is not due again: still no suggestion.
	again := long
	again.State.BreakFrom, again.State.BreakAt = at(8, 30, 0), at(9, 25, 0)
	if f, b := kinds(again); f != 0 || b != 0 {
		t.Errorf("5 min after a break reminder: %d focus, %d break; want none", f, b)
	}
	// The end-of-session prompt sets the same state.
	prompt := recBase()
	prompt.State.BreakFrom, prompt.State.BreakAt = at(9, 0, 0), at(9, 20, 0)
	if f, _ := kinds(prompt); f != 0 {
		t.Errorf("10 min after the break prompt: %d focus suggestions, want 0", f)
	}
	// A reminder 20 min ago no longer holds a suggestion back.
	old := recBase()
	old.State.BreakFrom, old.State.BreakAt = at(9, 0, 0), at(9, 10, 0)
	if f, _ := kinds(old); f != 1 {
		t.Errorf("20 min after a break reminder: %d focus suggestions, want 1", f)
	}

	// The tick that says "Break over", and the 15 min after it.
	over := recBase()
	over.State.BreakTakenAt, over.State.BreakUntil = at(9, 20, 0), at(9, 29, 0)
	if f, _ := kinds(over); f != 0 {
		t.Errorf("tick of Break over: %d focus suggestions, want 0", f)
	}
	if evs := only(Step(over), Notify); len(evs) != 1 || evs[0].Topic != TopicBreakOver {
		t.Errorf("Break over notice = %+v", evs)
	}
	over.State.BreakTakenAt, over.State.BreakUntil, over.State.BreakOverAt = at(9, 0, 0), at(9, 10, 0), at(9, 10, 0)
	if f, _ := kinds(over); f != 1 {
		t.Errorf("20 min after Break over: %d focus suggestions, want 1", f)
	}
}
