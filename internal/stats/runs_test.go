package stats

import (
	"testing"

	"tock/internal/span"
)

func TestLongestRun(t *testing.T) {
	tests := []struct {
		name string
		segs []seg
		want int64
	}{
		{"empty", nil, 0},
		{"one kind", []seg{deep(30 * mn), deep(20 * mn)}, 50 * mn},
		{"a short stretch joins the run", []seg{deep(20 * mn), distract(20 * sec), deep(10 * mn)}, 30*mn + 20*sec},
		{"a switch starts a new run", []seg{deep(20 * mn), shallow(30 * mn), deep(10 * mn)}, 30 * mn},
		{"idle ends the run", []seg{deep(20 * mn), idle(10 * mn), deep(15 * mn)}, 20 * mn},
		{"a long gap ends the run", []seg{deep(20 * mn), after(5*mn, deep(25*mn))}, 25 * mn},
		{"a short gap does not", []seg{deep(20 * mn), after(mn, deep(25*mn))}, 45 * mn},
		{"short stretches before the first long one belong to it",
			[]seg{shallow(10 * sec), distract(10 * sec), deep(10 * mn)}, 10*mn + 20*sec},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs, ls := at(tt.segs...)
			if got := Compute(vs, ls).LongestRunMS; got != tt.want {
				t.Fatalf("LongestRunMS = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStretchTerm(t *testing.T) {
	w := Weights{Depth: 0.4, Blocks: 0.3, Calm: 0.2, Stretch: 0.1}
	d := Day{ActiveMS: 3 * 60 * mn, LongestRunMS: 45 * mn}
	s := ScoreDay(d, target4h, w)
	if !near(s.Stretch, 0.5) {
		t.Fatalf("Stretch = %v, want 0.5", s.Stretch)
	}
	d.LongestRunMS = 3 * StretchFullMS
	if s := ScoreDay(d, target4h, w); !near(s.Stretch, 1) {
		t.Fatalf("Stretch = %v, want 1 (capped)", s.Stretch)
	}
	// calm 1 -> 20, stretch 1 -> 10
	if s := ScoreDay(d, target4h, w); s.Value != 30 {
		t.Fatalf("Value = %d, want 30", s.Value)
	}
	if s := ScoreDay(d, target4h, DefaultWeights); s.Value != 20 {
		t.Fatalf("default weights: Value = %d, want 20 (stretch weight 0)", s.Value)
	}
}

func TestAwayActsAsBreak(t *testing.T) {
	vs, ls := at(deep(20*mn), shallow(30*mn), deep(20*mn))
	vs[1].BundleID, vs[1].App = span.AwayBundleID, span.AwayApp // a labelled lunch
	d := Compute(vs, ls)
	if d.ActiveMS != 70*mn || d.DeepMS != 40*mn {
		t.Fatalf("totals: active %d deep %d", d.ActiveMS, d.DeepMS)
	}
	if d.Switches != 0 || d.LongestRunMS != 20*mn || len(d.Blocks) != 2 {
		t.Fatalf("away must break: %d switches, run %d, %d blocks", d.Switches, d.LongestRunMS, len(d.Blocks))
	}
	vs[1].BundleID = "app"
	if d := Compute(vs, ls); d.Switches != 2 {
		t.Fatalf("screen time between: %d switches", d.Switches)
	}
}
