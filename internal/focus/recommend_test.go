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
