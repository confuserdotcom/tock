package focus

import (
	"testing"

	"tock/internal/classify"
)

// Focus off: no break reminder, no session step, no auto session and
// no guard; the start-of-day notice still goes out.
func TestFocusOff(t *testing.T) {
	deep := classify.KindDeep
	in := input(at(14, 50, 0), vw{at(13, 0, 0), at(14, 50, 0), deep, "", ""})
	in.Sessions = []Session{{ID: 1, Kind: KindManual, StartMS: at(14, 0, 0), PlannedMS: at(14, 40, 0)}}
	if len(only(Step(in), Notify)) == 0 || len(only(Step(in), AutoSession)) == 0 && len(only(Step(in), SessionEnd)) == 0 {
		t.Fatal("focus on: the fixture should prompt something")
	}
	in.Cfg.Enabled = false
	for _, ev := range Step(in) {
		switch ev.Kind {
		case SessionUpdate, SessionEnd, AutoSession:
			t.Fatalf("focus off wrote %+v", ev)
		case Notify:
			if ev.Topic == TopicBreak || ev.Topic == TopicFocus || ev.OfSession {
				t.Fatalf("focus off sent %+v", ev)
			}
		}
	}
	if g := GuardStep(in, GuardState{}); g.Start != nil || g.Block != nil || g.Changed {
		t.Fatalf("focus off guard %+v", g)
	}
	// A hook started before Focus went off is stopped.
	if g := GuardStep(in, GuardState{HookSession: 1}); !g.Stop || g.State.HookSession != 0 {
		t.Fatalf("focus off left the hook running: %+v", g)
	}

	in = startInput(at(9, 30, 0), vw{at(9, 0, 0), at(9, 30, 0), deep, "", ""})
	in.StartBody = "S."
	in.Cfg.Enabled = false
	start := false
	for _, ev := range only(Step(in), Notify) {
		start = start || ev.Topic == TopicStart
	}
	if !start {
		t.Fatal("focus off stopped the start-of-day notice")
	}
}
