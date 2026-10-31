package focus

import (
	"fmt"
	"math"
)

// Weights are the shares of the four quality parts in the score.
type Weights struct{ Share, Longest, Calm, Steady float64 }

// DefaultQualityWeights is the shipped weighting of the four parts.
func DefaultQualityWeights() Weights {
	return Weights{Share: 0.45, Longest: 0.25, Calm: 0.15, Steady: 0.15}
}

// LongestFullMS is the longest deep stretch that earns the longest part in
// full: a session shorter than this needs one stretch as long as itself.
const LongestFullMS = 50 * 60_000

// Part is one scored part of a session, 0..100.
type Part struct {
	Value int
	NA    bool // no data (a session from before migration 0014 has no switch count); left out of the score
	Auto  bool // the calm part of an auto session: no interruption count exists, counted as 1.0
}

// QualityScore is a session's 0..100 score and its four parts in the fixed
// order share, longest, calm, steady.
type QualityScore struct {
	Score int
	Parts [4]Part
}

// Components returns the four parts of s as fractions in [0,1], in the order
// share, longest, calm, steady. steadyKnown is false for a session from
// before migration 0014 (Switches < 0). A session of no length scores all
// zero.
func Components(s Session) (x [4]float64, steadyKnown bool) {
	steadyKnown = s.Switches >= 0
	length := s.EndMS - s.StartMS
	if length <= 0 {
		return x, steadyKnown
	}
	x[0] = float64(s.DeepMS) / float64(length)
	x[1] = float64(s.LongestMS) / float64(min(length, LongestFullMS))
	x[2] = 1 / float64(1+s.Interruptions)
	if s.Kind == KindAuto {
		x[2] = 1
	}
	perHalfHour := float64(max(s.Switches, 0)) * 1_800_000 / float64(length)
	x[3] = 1 / (1 + perHalfHour)
	for i := range x {
		x[i] = min(max(x[i], 0), 1)
	}
	return x, steadyKnown
}

// Quality scores s with the weights w; the zero Weights value means
// DefaultQualityWeights(). It is computed from the stored counters each time,
// so changed weights apply to old sessions too. A session from before
// migration 0014 has no steady part: it is left out and the other weights are
// scaled to fill its share.
func Quality(s Session, w Weights) QualityScore {
	if w == (Weights{}) {
		w = DefaultQualityWeights()
	}
	x, steadyKnown := Components(s)
	var q QualityScore
	for i := range x {
		q.Parts[i].Value = int(math.Round(100 * x[i]))
	}
	q.Parts[2].Auto = s.Kind == KindAuto
	if !steadyKnown {
		q.Parts[3] = Part{NA: true}
	}
	if s.EndMS-s.StartMS <= 0 {
		return q
	}
	ws := [4]float64{w.Share, w.Longest, w.Calm, w.Steady}
	n := len(x)
	if !steadyKnown {
		n = 3
	}
	var sum, total float64
	for i := 0; i < n; i++ {
		sum += float64(ws[i] * x[i])
		total += ws[i]
	}
	if !steadyKnown && total > 0 {
		sum /= total
	}
	q.Score = int(min(max(math.Round(100*sum), 0), 100))
	return q
}

// Line is the breakdown of the score, parts in the fixed order share,
// longest, calm, steady: "share 82 · longest 70 · calm 50 · steady 36".
func (q QualityScore) Line() string {
	calm := fmt.Sprintf("calm %d", q.Parts[2].Value)
	if q.Parts[2].Auto {
		calm = "calm n/a (auto)"
	}
	steady := fmt.Sprintf("steady %d", q.Parts[3].Value)
	if q.Parts[3].NA {
		steady = "steady n/a"
	}
	return fmt.Sprintf("share %d · longest %d · %s · %s", q.Parts[0].Value, q.Parts[1].Value, calm, steady)
}

// InterruptionsText is "no interruptions", "1 interruption" or "3
// interruptions".
func InterruptionsText(n int) string {
	switch n {
	case 0:
		return "no interruptions"
	case 1:
		return "1 interruption"
	}
	return fmt.Sprintf("%d interruptions", n)
}
