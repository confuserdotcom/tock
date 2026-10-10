package focus

import (
	"testing"

	"tock/internal/classify"
)

func guardInput(now int64, vs ...vw) Input {
	in := input(now, vs...)
	in.Sessions = []Session{running()}
	in.Cfg.Block = true
	return in
}

func TestGuardHooks(t *testing.T) {
	in := input(at(14, 1, 0), vw{at(14, 0, 0), at(14, 1, 0), classify.KindDeep, "", ""})
	if g := GuardStep(in, GuardState{}); g.Start != nil || g.Stop || g.Changed {
		t.Fatalf("no session: %+v", g)
	}
	in.Sessions = []Session{running()}
	g := GuardStep(in, GuardState{})
	if g.Start == nil || g.Start.ID != 7 || g.Stop || !g.Changed || g.State.HookSession != 7 {
		t.Fatalf("start: %+v", g)
	}
	// The next tick, and a restarted trackd, see the saved state: nothing.
	if g := GuardStep(in, g.State); g.Start != nil || g.Stop || g.Changed {
		t.Fatalf("again: %+v", g)
	}
	// Stopped by tock: no open session.
	in.Sessions[0].EndMS = at(14, 1, 0)
	if g := GuardStep(in, GuardState{HookSession: 7}); g.Start != nil || !g.Stop || g.State.HookSession != 0 {
		t.Fatalf("stop: %+v", g)
	}
	// Planned end reached: stop in the same tick Step ends it.
	in = input(at(14, 25, 0), vw{at(14, 0, 0), at(14, 25, 0), classify.KindDeep, "", ""})
	in.Sessions = []Session{running()}
	if g := GuardStep(in, GuardState{HookSession: 7}); !g.Stop || g.Start != nil {
		t.Fatalf("planned end: %+v", g)
	}
	// A session that ended unseen, then another one.
	in = input(at(15, 1, 0), vw{at(15, 0, 0), at(15, 1, 0), classify.KindDeep, "", ""})
	s := running()
	s.ID, s.StartMS, s.PlannedMS = 8, at(15, 0, 0), at(15, 50, 0)
	in.Sessions = []Session{s}
	if g := GuardStep(in, GuardState{HookSession: 7}); !g.Stop || g.Start == nil || g.Start.ID != 8 {
		t.Fatalf("replaced: %+v", g)
	}
}

func TestGuardBlocksAfterGrace(t *testing.T) {
	deep := vw{at(14, 0, 0), at(14, 5, 0), classify.KindDeep, "", ""}
	yt := func(now int64) Input {
		return guardInput(now, deep, vw{at(14, 5, 0), now, classify.KindDistract, "com.google.Chrome", "www.youtube.com/watch"})
	}
	hooked := GuardState{HookSession: 7}
	if g := GuardStep(yt(at(14, 5, 9)), hooked); g.Block != nil {
		t.Fatalf("inside grace: %+v", g.Block)
	}
	g := GuardStep(yt(at(14, 5, 10)), hooked)
	if g.Block == nil || g.Block.Host != "www.youtube.com" || g.Block.Bundle != "com.google.Chrome" ||
		g.Block.LeftMS != at(14, 25, 0)-at(14, 5, 10) || !g.Changed || g.State.BlockAt != at(14, 5, 10) {
		t.Fatalf("block = %+v", g)
	}
	// Not again within the grace after a block.
	if g2 := GuardStep(yt(at(14, 5, 15)), g.State); g2.Block != nil {
		t.Fatalf("repeat too soon: %+v", g2.Block)
	}
	if g2 := GuardStep(yt(at(14, 5, 20)), g.State); g2.Block == nil {
		t.Fatal("no repeat after the grace")
	}
	// Off, in a call, or without a session: nothing.
	in := yt(at(14, 6, 0))
	in.Cfg.Block = false
	if g := GuardStep(in, hooked); g.Block != nil {
		t.Fatal("blocked with block off")
	}
	in = yt(at(14, 6, 0))
	in.Call = true
	if g := GuardStep(in, hooked); g.Block != nil {
		t.Fatal("blocked in a call")
	}
	in = yt(at(14, 6, 0))
	in.Sessions = nil
	if g := GuardStep(in, GuardState{}); g.Block != nil {
		t.Fatal("blocked without a session")
	}
	// Deep work in front: nothing.
	if g := GuardStep(guardInput(at(14, 6, 0), deep, vw{at(14, 5, 0), at(14, 6, 0), classify.KindDeep, "", ""}), hooked); g.Block != nil {
		t.Fatal("blocked deep work")
	}
}

func TestBlockable(t *testing.T) {
	u := func(s string) *string { return &s }
	cases := []struct {
		v    classify.SpanView
		kind string
		want bool
	}{
		{classify.SpanView{BundleID: "x"}, classify.KindDistract, true},
		{classify.SpanView{BundleID: "x"}, classify.KindDeep, false},
		{classify.SpanView{BundleID: "com.valve.steam"}, "", true},
		{classify.SpanView{BundleID: "b", URL: u("m.youtube.com:443/x")}, "", true},
		{classify.SpanView{BundleID: "b", URL: u("notyoutube.com/x")}, "", false},
		{classify.SpanView{BundleID: "tock.away"}, classify.KindDistract, false},
		{classify.SpanView{BundleID: "com.apple.finder"}, classify.KindDistract, false},
		{classify.SpanView{ID: -(classify.RemoteIDBase + 1), BundleID: "x"}, classify.KindDistract, false},
	}
	for _, c := range cases {
		if got := Blockable(c.v, classify.Label{Kind: c.kind}, []string{"com.valve.steam"}, []string{"youtube.com"}); got != c.want {
			t.Errorf("Blockable(%+v, %q) = %v", c.v, c.kind, got)
		}
	}
}
