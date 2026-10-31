package focus

import (
	"testing"

	"tock/internal/classify"
	"tock/internal/config"
)

func TestWindowStatsAlertMS(t *testing.T) {
	deep, dis := classify.KindDeep, classify.KindDistract
	in := input(at(10, 40, 0),
		vw{at(10, 0, 0), at(10, 10, 0), deep, "", ""},
		vw{at(10, 10, 0), at(10, 14, 0), dis, "", ""},                  // quiet
		vw{at(10, 14, 0), at(10, 16, 0), dis, "", ""},                  // alertable
		vw{at(10, 16, 0), at(10, 20, 0), dis, "us.zoom.xos", ""},       // meeting
		vw{at(10, 20, 0), at(10, 30, 0), "idle", "", ""},               // idle
		vw{at(10, 30, 0), at(10, 31, 0), dis, "", "meet.google.com/x"}, // meeting host
	)
	in.Labels[1].Quiet = true
	got := WindowStats(in.Views, in.Labels, at(10, 0, 0), at(11, 0, 0), config.DefaultFocus())
	want := Stats{DeepMS: 10 * minute, DistractMS: 6 * minute, AlertMS: 2 * minute, LongestMS: 10 * minute}
	if got != want {
		t.Fatalf("WindowStats = %+v, want %+v", got, want)
	}
}

// quietRun is a session with 5 min of deep work, then distraction since
// 14:05 that is marked quiet when quiet is set.
func quietRun(now int64, quiet bool, s Session) Input {
	in := snoozedRun(now, 0, s)
	mark(&in, 1, "Entertainment", classify.SourceRule, "rules.toml#3", quiet)
	return in
}

func TestStepQuietNeverAlerts(t *testing.T) {
	s := running()
	// 9 min of quiet distraction: far past the grace and one repeat.
	for _, now := range []int64{at(14, 6, 10), at(14, 9, 0), at(14, 14, 0)} {
		evs := Step(quietRun(now, true, s))
		if n := only(evs, Notify); len(n) != 0 {
			t.Fatalf("tick %d: quiet distraction notified: %+v", now, n)
		}
		if u := only(evs, SessionUpdate); len(u) == 1 {
			s = u[0].Sess
		}
	}
	if s.Interruptions != 0 || s.AlertedMS != 0 {
		t.Fatalf("session = %+v, want no interruptions and AlertedMS 0", s)
	}
	if s.DistractMS != 9*minute {
		t.Fatalf("DistractMS = %d, want %d (marked time still counts)", s.DistractMS, 9*minute)
	}
	// The same time unmarked alerts, and its distraction minutes match.
	evs := Step(quietRun(at(14, 14, 0), false, running()))
	if len(only(evs, Notify)) != 1 {
		t.Fatalf("unmarked run = %+v, want one notice", evs)
	}
	if u := sessionUpdate(t, evs); u.DistractMS != s.DistractMS {
		t.Fatalf("DistractMS marked %d, unmarked %d, want the same", s.DistractMS, u.DistractMS)
	}
}

func TestStepUnmarkNoImmediateAlert(t *testing.T) {
	// 5 min of distraction were alerted on. The first 5 min of it are then
	// marked quiet, so the alertable total drops to what came after.
	marked := func(now int64, s Session) Input {
		in := input(now,
			vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""},
			vw{at(14, 5, 0), at(14, 10, 0), classify.KindDistract, "", ""},
			vw{at(14, 10, 0), now, classify.KindDistract, "", ""})
		in.Sessions = []Session{s}
		mark(&in, 1, "Entertainment", classify.SourceRule, "rules.toml#3", true)
		mark(&in, 2, "Games", classify.SourceRule, "rules.toml#5", false)
		return in
	}
	s := running()
	s.Interruptions, s.AlertedMS = 1, 5*minute
	evs := Step(marked(at(14, 11, 0), s))
	if n := only(evs, Notify); len(n) != 0 {
		t.Fatalf("after the mark: %+v, want no notice", n)
	}
	s = sessionUpdate(t, evs)
	if s.AlertedMS != minute || s.Interruptions != 1 || s.DistractMS != 6*minute {
		t.Fatalf("after the mark: %+v, want AlertedMS 1 min, Interruptions 1, DistractMS 6 min", s)
	}
	// The baseline followed the total down: the next notice is one repeat
	// after 1 min of alertable time, not one repeat after the stale 5 min.
	if n := only(Step(marked(at(14, 12, 50), s)), Notify); len(n) != 0 {
		t.Fatalf("2:50 alertable: %+v, want none", n)
	}
	evs = Step(marked(at(14, 13, 0), s))
	if n := only(evs, Notify); len(n) != 1 {
		t.Fatalf("3:00 alertable: %+v, want one notice", evs)
	}
	if u := sessionUpdate(t, evs); u.Interruptions != 2 || u.AlertedMS != 3*minute {
		t.Fatalf("after the notice: %+v, want Interruptions 2 and AlertedMS 3 min", u)
	}
}

func TestStepNoticeCarriesTarget(t *testing.T) {
	evs := Step(quietRun(at(14, 6, 10), false, running()))
	n := only(evs, Notify)
	if len(n) != 1 || n[0].Topic != TopicFocus {
		t.Fatalf("events = %+v, want one TopicFocus notice", evs)
	}
	if n[0].Category != "Entertainment" || n[0].RuleRef != "rules.toml#3" {
		t.Fatalf("notice target = (%q, %q), want (Entertainment, rules.toml#3)", n[0].Category, n[0].RuleRef)
	}
}

func TestGuardBlocksQuiet(t *testing.T) {
	v := classify.SpanView{BundleID: "com.google.Chrome"}
	if !Blockable(v, classify.Label{Kind: classify.KindDistract, Category: "Entertainment", Quiet: true}, nil, nil) {
		t.Fatal("a quiet distraction must still be blockable: the mark only affects notices")
	}
	in := guardInput(at(14, 5, 10),
		vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""},
		vw{at(14, 5, 0), at(14, 5, 10), classify.KindDistract, "com.google.Chrome", "www.youtube.com/watch"})
	in.Labels[1].Quiet = true
	if g := GuardStep(in, GuardState{HookSession: 7}); g.Block == nil {
		t.Fatal("GuardStep did not block a quiet distraction with block = true")
	}
}
