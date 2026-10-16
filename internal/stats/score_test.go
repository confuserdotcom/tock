package stats

import (
	"math"
	"testing"
)

var target4h = Target{MS: 4 * 60 * mn}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestScoreWorkedExample(t *testing.T) {
	// The spec's breakdown example: focus 2h10 of 4h, 1h20 in long blocks,
	// 14 switches in 3 h active, 12 min distraction.
	d := Day{ActiveMS: 3 * 60 * mn, FocusMS: 130 * mn, LongMS: 80 * mn, Switches: 14, DistractMS: 12 * mn}
	s := ScoreDay(d, target4h, DefaultWeights)
	if !near(s.Depth, 130.0/240) || !near(s.Blocks, 80.0/130) || !near(s.SwitchesPerHour, 14.0/3) ||
		!near(s.Calm, 1-14.0/3/20) || s.Penalty != 3 {
		t.Fatalf("terms = %+v", s)
	}
	// 100 * (0.5*0.54167 + 0.3*0.61538 + 0.2*0.76667) = 60.88 -> 61, minus 3
	if s.Value != 58 {
		t.Fatalf("Value = %d, want 58", s.Value)
	}
	if s.Target != target4h || s.Weights != DefaultWeights {
		t.Fatalf("Target/Weights not carried: %+v", s)
	}
}

func TestScoreEdges(t *testing.T) {
	tests := []struct {
		name string
		d    Day
		want int
	}{
		{"empty day", Day{}, 0},
		{"all distraction: penalty capped at 15",
			Day{ActiveMS: 3 * 60 * mn, DistractMS: 3 * 60 * mn}, 5}, // calm 1 -> 20, penalty 15
		{"calm alone cannot rescue switches", Day{ActiveMS: 60 * mn, Switches: 20}, 0},
		{"over target: depth is 1",
			Day{ActiveMS: 6 * 60 * mn, FocusMS: 5 * 60 * mn, LongMS: 5 * 60 * mn}, 100},
		{"no focus: blocks term is 0", Day{ActiveMS: 60 * mn}, 20},
		{"short morning: active hours floored at 1",
			Day{ActiveMS: 10 * mn, Switches: 3}, 17}, // 3 an hour -> calm 0.85
		{"penalty rounds distraction down to whole minutes",
			Day{ActiveMS: 60 * mn, DistractMS: 8*mn - 1}, 19}, // 7 min -> 1 point
		{"penalty cap", Day{ActiveMS: 60 * mn, DistractMS: 60 * mn}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScoreDay(tt.d, target4h, DefaultWeights).Value; got != tt.want {
				t.Fatalf("Value = %d, want %d (%+v)", got, tt.want, ScoreDay(tt.d, target4h, DefaultWeights))
			}
		})
	}
}

func TestScoreClampsAtZero(t *testing.T) {
	d := Day{ActiveMS: 60 * mn, Switches: 40, DistractMS: 60 * mn}
	if s := ScoreDay(d, target4h, DefaultWeights); s.Value != 0 || s.Penalty != PenaltyCap {
		t.Fatalf("score = %+v, want value 0 with penalty %d", s, PenaltyCap)
	}
}

func TestScoreEmptyDayHasNoTerms(t *testing.T) {
	s := ScoreDay(Day{}, target4h, DefaultWeights)
	if s.Depth != 0 || s.Blocks != 0 || s.Calm != 0 || s.Sum() != 0 {
		t.Fatalf("empty day terms = %+v", s)
	}
}

func TestScoreWeights(t *testing.T) {
	d := Day{ActiveMS: 60 * mn} // depth 0, blocks 0, calm 1
	if got := ScoreDay(d, target4h, Weights{Depth: 0.4, Blocks: 0.2, Calm: 0.4}).Value; got != 40 {
		t.Fatalf("calm weight 0.4: Value = %d, want 40", got)
	}
}

func TestScoreZeroTarget(t *testing.T) {
	d := Day{ActiveMS: 60 * mn, FocusMS: 30 * mn, LongMS: 30 * mn}
	if s := ScoreDay(d, Target{}, DefaultWeights); s.Depth != 0 {
		t.Fatalf("zero target: Depth = %v, want 0", s.Depth)
	}
}

func TestBrowseWeighsAThird(t *testing.T) {
	d := Day{ActiveMS: 3_600_000, BrowseMS: 36 * 60_000}
	if s := ScoreDay(d, target4h, DefaultWeights); s.Penalty != 3 {
		t.Fatalf("36m browsing: penalty %d, want 3 (12 distraction minutes)", s.Penalty)
	}
	d.DistractMS = 8 * 60_000
	if s := ScoreDay(d, target4h, DefaultWeights); s.Penalty != 5 {
		t.Fatalf("plus 8m distraction: penalty %d, want 5", s.Penalty)
	}
}
