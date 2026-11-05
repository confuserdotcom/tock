package focus

import (
	"testing"

	"tock/internal/classify"
)

// wakeSess is a 50 min session from 14:00.
func wakeSess() Session {
	return Session{ID: 9, Kind: KindManual, StartMS: at(14, 0, 0), PlannedMS: at(14, 50, 0)}
}

// wakeIn is a tick at now in wakeSess with the wake screen on and the views
// vs; view 0 is labelled Study (deep) when it is deep.
func wakeIn(now int64, st State, s Session, vs ...vw) Input {
	in := input(now, vs...)
	in.Cfg.WakeScreen = true
	in.Sessions = []Session{s}
	in.State = st
	for i := range in.Labels {
		if in.Labels[i].Kind == classify.KindDeep {
			in.Labels[i].Category = "Study"
		}
	}
	return in
}

func topic(evs []Event, tp string) []Event {
	var out []Event
	for _, e := range evs {
		if e.Kind == Notify && e.Topic == tp {
			out = append(out, e)
		}
	}
	return out
}

func lastState(t *testing.T, evs []Event) State {
	t.Helper()
	u := only(evs, StateUpdate)
	if len(u) == 0 {
		t.Fatalf("no StateUpdate in %+v", evs)
	}
	return u[len(u)-1].State
}

var (
	deep    = classify.KindDeep
	shallow = classify.KindShallow
	browse  = classify.KindBrowse
	dis     = classify.KindDistract
	neutral = classify.KindNeutral
)

func TestOffTaskRun(t *testing.T) {
	s := wakeSess()
	for _, c := range []struct {
		name  string
		paper bool
		vs    []vw
		off   int64 // counted ms
		from  int64
		front bool // the front view is off task
	}{
		{"distraction after work", false, []vw{
			{at(14, 0, 0), at(14, 10, 0), deep, "", ""},
			{at(14, 10, 0), at(14, 20, 0), dis, "", ""}}, 10 * minute, at(14, 10, 0), true},
		{"browse counts", false, []vw{
			{at(14, 0, 0), at(14, 10, 0), deep, "", ""},
			{at(14, 10, 0), at(14, 16, 0), browse, "", ""}}, 6 * minute, at(14, 10, 0), true},
		{"neutral neither counts nor resets", false, []vw{
			{at(14, 0, 0), at(14, 5, 0), dis, "", ""},
			{at(14, 5, 0), at(14, 15, 0), neutral, "", ""},
			{at(14, 15, 0), at(14, 20, 0), dis, "", ""}}, 10 * minute, at(14, 0, 0), true},
		{"a work glance under a minute does not reset", false, []vw{
			{at(14, 0, 0), at(14, 5, 0), dis, "", ""},
			{at(14, 5, 0), at(14, 5, 50), shallow, "", ""},
			{at(14, 5, 50), at(14, 10, 50), dis, "", ""}}, 10 * minute, at(14, 0, 0), true},
		{"a minute of work resets", false, []vw{
			{at(14, 0, 0), at(14, 5, 0), dis, "", ""},
			{at(14, 5, 0), at(14, 6, 0), shallow, "", ""},
			{at(14, 6, 0), at(14, 8, 0), dis, "", ""}}, 2 * minute, at(14, 6, 0), true},
		{"short idle neither", false, []vw{
			{at(14, 0, 0), at(14, 5, 0), dis, "", ""},
			{at(14, 5, 0), at(14, 15, 0), "idle", "", ""},
			{at(14, 15, 0), at(14, 17, 0), dis, "", ""}}, 7 * minute, at(14, 0, 0), true},
		{"long idle counts whole", false, []vw{
			{at(14, 0, 0), at(14, 5, 0), deep, "", ""},
			{at(14, 5, 0), at(14, 21, 0), "idle", "", ""}}, 16 * minute, at(14, 5, 0), true},
		{"idle under the mark is not off task yet", false, []vw{
			{at(14, 0, 0), at(14, 5, 0), deep, "", ""},
			{at(14, 5, 0), at(14, 15, 0), "idle", "", ""}}, 0, 0, false},
		{"on paper idle never counts", true, []vw{
			{at(14, 0, 0), at(14, 5, 0), deep, "", ""},
			{at(14, 5, 0), at(14, 30, 0), "idle", "", ""}}, 0, 0, false},
		{"meeting neither", false, []vw{
			{at(14, 0, 0), at(14, 5, 0), dis, "", ""},
			{at(14, 5, 0), at(14, 9, 0), deep, "us.zoom.xos", ""}}, 5 * minute, at(14, 0, 0), false},
		{"quiet distraction neither", false, nil, 0, 0, false},
		{"back at work", false, []vw{
			{at(14, 0, 0), at(14, 10, 0), dis, "", ""},
			{at(14, 10, 0), at(14, 10, 30), deep, "", ""}}, 10 * minute, at(14, 0, 0), false},
	} {
		vs := c.vs
		if vs == nil {
			vs = []vw{{at(14, 0, 0), at(14, 12, 0), dis, "", ""}}
		}
		in := wakeIn(vs[len(vs)-1].to, State{}, s, vs...)
		if c.name == "quiet distraction neither" {
			in.Labels[0].Quiet = true
		}
		r := offTask(in, s, c.paper)
		if r.OffMS != c.off || r.From != c.from || r.Front != c.front {
			t.Errorf("%s: run = %+v, want off %d from %d front %v", c.name, r, c.off, c.from, c.front)
		}
	}
}

func TestWakeSteps(t *testing.T) {
	s := wakeSess()
	work := vw{at(14, 0, 0), at(14, 10, 0), deep, "", ""}
	browsing := func(now int64) vw { return vw{at(14, 10, 0), now, browse, "Safari", "news.example.com/a"} }

	// 4 min: nothing.
	if evs := Step(wakeIn(at(14, 14, 0), State{}, s, work, browsing(at(14, 14, 0)))); len(topic(evs, TopicWakeWarn))+len(topic(evs, TopicWake)) != 0 {
		t.Fatalf("early: %+v", evs)
	}
	// 5 min: the banner, once.
	evs := Step(wakeIn(at(14, 15, 0), State{}, s, work, browsing(at(14, 15, 0))))
	w := topic(evs, TopicWakeWarn)
	if len(w) != 1 || w[0].Body != "5 min off task · 35 min left." || !w[0].OfSession {
		t.Fatalf("warn = %+v", evs)
	}
	if w[0].Target != (classify.Want{Category: "Study"}) || w[0].Key != "Safari · news.example.com" {
		t.Fatalf("warn target %+v key %q", w[0].Target, w[0].Key)
	}
	st := lastState(t, evs)
	if evs := Step(wakeIn(at(14, 16, 0), st, s, work, browsing(at(14, 16, 0)))); len(topic(evs, TopicWakeWarn)) != 0 {
		t.Fatal("warned twice")
	}
	// 10 min: the curtain.
	evs = Step(wakeIn(at(14, 20, 0), st, s, work, browsing(at(14, 20, 0))))
	c := topic(evs, TopicWake)
	if len(c) != 1 || c[0].OffFrom != at(14, 10, 0) || c[0].Title != TitleWake ||
		c[0].Body != "10 min off task · 30 min left of your 50 min session." {
		t.Fatalf("curtain = %+v", evs)
	}
	if u := sessionUpdate(t, evs); u.Wakes != 1 {
		t.Fatalf("wakes = %d", u.Wakes)
	}
	st = lastState(t, evs)
	if st.WakeCheck != at(14, 20, 0) {
		t.Fatalf("check = %d", st.WakeCheck)
	}
	s.Wakes = 1
	// Not again before another wake_after.
	if evs := Step(wakeIn(at(14, 25, 0), st, s, work, browsing(at(14, 25, 0)))); len(topic(evs, TopicWake)) != 0 {
		t.Fatal("curtain again too soon")
	}
	evs = Step(wakeIn(at(14, 30, 0), st, s, work, browsing(at(14, 30, 0))))
	if len(topic(evs, TopicWake)) != 1 {
		t.Fatalf("no second curtain: %+v", evs)
	}
	st = lastState(t, evs)
	s.Wakes = 2
	// Two a session at most.
	if evs := Step(wakeIn(at(14, 40, 0), st, s, work, browsing(at(14, 40, 0)))); len(topic(evs, TopicWake)) != 0 {
		t.Fatal("third curtain")
	}
}

func TestWakeBackCheck(t *testing.T) {
	s := wakeSess()
	s.Wakes = 1
	st := State{WakeRun: at(14, 10, 0), WakeAt: at(14, 20, 0), WakeCheck: at(14, 20, 0), WakeWarn: at(14, 15, 0)}
	vs := []vw{{at(14, 0, 0), at(14, 10, 0), deep, "", ""}, {at(14, 10, 0), at(14, 20, 0), browse, "", ""}}
	back := append(vs, vw{at(14, 20, 0), at(14, 21, 0), deep, "", ""})
	evs := Step(wakeIn(at(14, 21, 0), st, s, back...))
	if u := sessionUpdate(t, evs); u.WakesBack != 1 {
		t.Fatalf("back = %+v", u)
	}
	if lastState(t, evs).WakeCheck != 0 {
		t.Fatal("check not cleared")
	}
	// Still browsing 2 min later: dropped, not counted.
	late := append(vs, vw{at(14, 20, 0), at(14, 22, 30), browse, "", ""})
	evs = Step(wakeIn(at(14, 22, 30), st, s, late...))
	for _, u := range only(evs, SessionUpdate) {
		if u.Sess.WakesBack != 0 {
			t.Fatal("counted back while off task")
		}
	}
	if lastState(t, evs).WakeCheck != 0 {
		t.Fatal("check kept past its window")
	}
}

func TestWakeHeldBack(t *testing.T) {
	s := wakeSess()
	vs := []vw{{at(14, 0, 0), at(14, 10, 0), deep, "", ""}, {at(14, 10, 0), at(14, 20, 0), browse, "", ""}}
	for _, c := range []struct {
		name string
		edit func(*Input)
	}{
		{"off", func(in *Input) { in.Cfg.WakeScreen = false }},
		{"call", func(in *Input) { in.Call = true }},
		{"snoozed", func(in *Input) { in.State.DistractSnoozeUntil = at(14, 25, 0) }},
	} {
		in := wakeIn(at(14, 20, 0), State{}, s, vs...)
		c.edit(&in)
		evs := Step(in)
		if len(topic(evs, TopicWake))+len(topic(evs, TopicWakeWarn)) != 0 {
			t.Errorf("%s: %+v", c.name, evs)
		}
	}
}

// A distraction already gets the distraction notice: no warning banner.
func TestWakeWarnSkipsDistraction(t *testing.T) {
	s := wakeSess()
	evs := Step(wakeIn(at(14, 15, 0), State{}, s,
		vw{at(14, 0, 0), at(14, 10, 0), deep, "", ""}, vw{at(14, 10, 0), at(14, 15, 0), dis, "", ""}))
	if len(topic(evs, TopicWakeWarn)) != 0 {
		t.Fatalf("warned over a distraction: %+v", evs)
	}
}

// Idle past wake_idle: the curtain comes straight away (no banner) and
// offers I'm on paper; a paper session never counts idle.
func TestWakeIdle(t *testing.T) {
	s := wakeSess()
	vs := []vw{{at(14, 0, 0), at(14, 5, 0), deep, "", ""}, {at(14, 5, 0), at(14, 20, 0), "idle", "", ""}}
	evs := Step(wakeIn(at(14, 20, 0), State{}, s, vs...))
	c := topic(evs, TopicWake)
	if len(c) != 1 || !c[0].Idle || c[0].Key != "" || len(topic(evs, TopicWakeWarn)) != 0 {
		t.Fatalf("idle curtain = %+v", evs)
	}
	in := wakeIn(at(14, 20, 0), State{PaperSession: s.ID}, s, vs...)
	if evs := Step(in); len(topic(evs, TopicWake)) != 0 {
		t.Fatal("curtain on paper")
	}
}
