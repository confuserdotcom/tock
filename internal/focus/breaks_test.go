package focus

import (
	"testing"

	"tock/internal/classify"
)

func TestBreaks(t *testing.T) {
	deep, neutral := classify.KindDeep, classify.KindNeutral
	withState := func(in Input, s State) Input { in.State = s; return in }
	withCamera := func(in Input) Input { in.Camera = true; return in }
	withSession := func(in Input) Input {
		in.Sessions = []Session{{ID: 1, Kind: KindManual, StartMS: at(13, 30, 0), PlannedMS: at(14, 20, 0)}}
		return in
	}
	tests := []struct {
		name  string
		in    Input
		body  string // "" = no reminder
		state State
	}{
		{"49 min", input(at(14, 49, 0), vw{at(14, 0, 0), at(14, 49, 0), deep, "", ""}), "", State{}},
		{"50 min", input(at(14, 50, 0), vw{at(14, 0, 0), at(14, 50, 0), deep, "", ""}),
			"50 min without a break. Stand up for 5 min.", State{BreakFrom: at(14, 0, 0), BreakAt: at(14, 50, 0)}},
		{"repeat waits", withState(input(at(14, 59, 0), vw{at(14, 0, 0), at(14, 59, 0), deep, "", ""}),
			State{BreakFrom: at(14, 0, 0), BreakAt: at(14, 50, 0)}), "", State{}},
		{"repeat after 10 min", withState(input(at(15, 0, 0), vw{at(14, 0, 0), at(15, 0, 0), deep, "", ""}),
			State{BreakFrom: at(14, 0, 0), BreakAt: at(14, 50, 0)}),
			"60 min without a break. Stand up for 5 min.", State{BreakFrom: at(14, 0, 0), BreakAt: at(15, 0, 0)}},
		{"new run after idle", withState(input(at(14, 56, 0),
			vw{at(13, 0, 0), at(14, 0, 0), deep, "", ""},
			vw{at(14, 0, 0), at(14, 6, 0), "idle", "", ""},
			vw{at(14, 6, 0), at(14, 56, 0), deep, "", ""}),
			State{BreakFrom: at(13, 0, 0), BreakAt: at(13, 55, 0)}),
			"50 min without a break. Stand up for 5 min.", State{BreakFrom: at(14, 6, 0), BreakAt: at(14, 56, 0)}},
		{"idle resets", input(at(14, 40, 0),
			vw{at(13, 0, 0), at(14, 0, 0), deep, "", ""},
			vw{at(14, 0, 0), at(14, 6, 0), "idle", "", ""},
			vw{at(14, 6, 0), at(14, 40, 0), deep, "", ""}), "", State{}},
		{"gap resets", input(at(14, 40, 0),
			vw{at(13, 0, 0), at(14, 0, 0), deep, "", ""},
			vw{at(14, 5, 0), at(14, 40, 0), deep, "", ""}), "", State{}},
		{"short gap does not reset", input(at(14, 0, 0),
			vw{at(13, 0, 0), at(13, 30, 0), deep, "", ""},
			vw{at(13, 34, 0), at(14, 0, 0), neutral, "", ""}),
			"60 min without a break. Stand up for 5 min.", State{BreakFrom: at(13, 0, 0), BreakAt: at(14, 0, 0)}},
		{"deferred in a session", withSession(input(at(14, 0, 0), vw{at(13, 0, 0), at(14, 0, 0), deep, "", ""})), "", State{}},
		{"meeting app", input(at(14, 55, 0), vw{at(14, 0, 0), at(14, 55, 0), "", "us.zoom.xos", ""}), "", State{}},
		{"camera", withCamera(input(at(14, 55, 0), vw{at(14, 0, 0), at(14, 55, 0), deep, "", ""})), "", State{}},
		{"nothing open now", input(at(14, 55, 0), vw{at(14, 0, 0), at(14, 50, 0), deep, "", ""}), "", State{}},
		{"idle now", input(at(14, 55, 0),
			vw{at(13, 0, 0), at(14, 0, 0), deep, "", ""},
			vw{at(14, 0, 0), at(14, 55, 0), "idle", "", ""}), "", State{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evs := Step(tt.in)
			notes, states := only(evs, Notify), only(evs, StateUpdate)
			if tt.body == "" {
				if len(notes) != 0 || len(states) != 0 {
					t.Fatalf("want no reminder, got %+v", evs)
				}
				return
			}
			if len(notes) != 1 || notes[0].Title != TitleBreak || notes[0].Body != tt.body || notes[0].OfSession {
				t.Fatalf("notes = %+v, want one %q", notes, tt.body)
			}
			if len(states) != 1 || states[0].State != tt.state {
				t.Fatalf("state = %+v, want %+v", states, tt.state)
			}
			if evs[len(evs)-1].Kind != Notify {
				t.Fatalf("notifications must come last: %+v", evs)
			}
		})
	}
}
