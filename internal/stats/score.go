package stats

import "math"

// Weights are the score's term weights; they sum to 1.
type Weights struct{ Depth, Blocks, Calm, Stretch float64 }

// DefaultWeights is the parent design's weighting.
var DefaultWeights = Weights{Depth: 0.5, Blocks: 0.3, Calm: 0.2}

// Score is a day's focus score with every term, for the breakdown.
type Score struct {
	Value           int     // 0..100
	Depth           float64 // min(FocusMS / Target.MS, 1)
	Blocks          float64 // LongMS / FocusMS, 0 when FocusMS is 0
	Calm            float64 // 1 - min(SwitchesPerHour / CalmZeroPerHour, 1)
	SwitchesPerHour float64 // Switches / max(ActiveMS in hours, 1)
	Stretch         float64 // min(LongestRunMS / StretchFullMS, 1)
	Penalty         int     // min(PenaltyCap, whole distraction minutes / PenaltyPerMin)
	Weights         Weights
	Target          Target
}

// Sum is the weighted sum of the terms, 0..100, before rounding and the
// penalty.
func (s Score) Sum() float64 {
	return 100 * (s.Weights.Depth*s.Depth + s.Weights.Blocks*s.Blocks + s.Weights.Calm*s.Calm + s.Weights.Stretch*s.Stretch)
}

// ScoreDay scores d against target t. A day with no active time scores 0
// with every term 0.
func ScoreDay(d Day, t Target, w Weights) Score {
	s := Score{Weights: w, Target: t}
	if d.ActiveMS <= 0 {
		return s
	}
	if t.MS > 0 {
		s.Depth = min(float64(d.FocusMS)/float64(t.MS), 1)
	}
	if d.FocusMS > 0 {
		s.Blocks = float64(d.LongMS) / float64(d.FocusMS)
	}
	s.SwitchesPerHour = float64(d.Switches) / max(float64(d.ActiveMS)/3_600_000, 1)
	s.Calm = 1 - min(s.SwitchesPerHour/CalmZeroPerHour, 1)
	s.Stretch = min(float64(d.LongestRunMS)/StretchFullMS, 1)
	s.Penalty = int(min(PenaltyCap, (d.DistractMS+d.BrowseMS/BrowseWeight)/60_000/PenaltyPerMin))
	s.Value = min(max(int(math.Round(s.Sum()))-s.Penalty, 0), 100)
	return s
}
