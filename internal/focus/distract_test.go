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

	// Unmarking makes the 6 min alertable again, but the user is back at
	// work: no notice and no interruption charged for time not lived now.
	back := func(now int64, s Session) Input {
		in := input(now,
			vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""},
			vw{at(14, 5, 0), at(14, 10, 0), classify.KindDistract, "", ""},
			vw{at(14, 10, 0), at(14, 11, 0), classify.KindDistract, "", ""},
			vw{at(14, 11, 0), now, classify.KindDeep, "", ""})
		in.Sessions = []Session{s}
		mark(&in, 1, "Entertainment", classify.SourceRule, "rules.toml#3", false)
		mark(&in, 2, "Games", classify.SourceRule, "rules.toml#5", false)
		return in
	}
	s = running()
	s.Interruptions, s.AlertedMS = 1, minute
	evs = Step(back(at(14, 11, 30), s))
	if n := only(evs, Notify); len(n) != 0 {
		t.Fatalf("after the unmark: %+v, want no notice", n)
	}
	for _, e := range only(evs, SessionUpdate) {
		if e.Sess.Interruptions != 1 {
			t.Fatalf("after the unmark: %+v, want Interruptions still 1", e.Sess)
		}
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

func urgeRun(now int64, snoozeUntil int64, s Session) Input {
	in := snoozedRun(now, snoozeUntil, s)
	in.Cfg.UrgeSurf = true
	return in
}

func TestStepUrgeBody(t *testing.T) {
	now := at(14, 10, 0)
	const plain = "5 min distracted in your 25 min session, 15 min left."
	evs := Step(urgeRun(now, 0, running()))
	n := only(evs, Notify)
	if len(n) != 1 || n[0].Topic != TopicFocus {
		t.Fatalf("events = %+v, want one TopicFocus notice", evs)
	}
	if want := "Pause. Breathe. Name the urge.\n" + plain; n[0].Body != want || !n[0].Urge {
		t.Fatalf("urge notice = (%q, urge %v), want (%q, true)", n[0].Body, n[0].Urge, want)
	}
	if UrgePrompt != "Pause. Breathe. Name the urge." {
		t.Fatalf("UrgePrompt = %q", UrgePrompt)
	}
	off := only(Step(snoozedRun(now, 0, running())), Notify)
	if len(off) != 1 || off[0].Body != plain || off[0].Urge {
		t.Fatalf("plain notice = %+v, want body %q and Urge false", off, plain)
	}
}

func TestStepUrgeSameTrigger(t *testing.T) {
	// Grace: 50 s of distraction is silent, 70 s alerts.
	if n := only(Step(urgeRun(at(14, 5, 50), 0, running())), Notify); len(n) != 0 {
		t.Fatalf("50 s: %+v, want no notice", n)
	}
	evs := Step(urgeRun(at(14, 6, 10), 0, running()))
	if n := only(evs, Notify); len(n) != 1 || !n[0].Urge {
		t.Fatalf("70 s: %+v, want one urge notice", evs)
	}
	// Repeat: after the first alert the next one comes two minutes later.
	s := sessionUpdate(t, evs)
	if n := only(Step(urgeRun(at(14, 7, 10), 0, s)), Notify); len(n) != 0 {
		t.Fatalf("1 min after the alert: %+v, want none", n)
	}
	if n := only(Step(urgeRun(at(14, 8, 10), 0, s)), Notify); len(n) != 1 || !n[0].Urge {
		t.Fatalf("2 min after the alert: %+v, want one urge notice", n)
	}
	// Snoozed: no notice, as in plain mode.
	now := at(14, 6, 10)
	if n := only(Step(urgeRun(now, now+1, running())), Notify); len(n) != 0 {
		t.Fatalf("snoozed: %+v, want none", n)
	}
	// Quiet: a marked distraction never alerts.
	in := urgeRun(at(14, 14, 0), 0, running())
	mark(&in, 1, "Entertainment", classify.SourceRule, "rules.toml#3", true)
	if n := only(Step(in), Notify); len(n) != 0 {
		t.Fatalf("quiet: %+v, want none", n)
	}
}
