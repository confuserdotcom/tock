package focus

import (
	"math"
	"slices"
)

// MinRated is how many eligible rated sessions the weights need before they
// are refit; below it the defaults stand.
const MinRated = 10

const (
	ridgeLambda = 2.0  // pull toward the default weights
	minWeight   = 0.05 // no part may drop out of the score
	maxWeight   = 0.70 // no part may take over the score
	snapUnit    = 1e9  // weights are rounded to 1e-9
)

// FitWeights learns the quality weights from the user's ratings. rated may
// hold any sessions; only finished ones rated 1..5 with a known switch count
// are used (a session from before migration 0014 has a made-up steady part
// and would bias the fit), and n is how many that is. Below MinRated it
// returns the defaults unchanged.
//
// The fit minimises Σ(y − w·x)² + λ‖w − w0‖² where x is Components of a
// session, y = (rating − 1)/4, w0 the default weights and λ = 2: a ridge
// regression without intercept that stays near the defaults until the ratings
// disagree with them. The four weights are then clamped to [0.05, 0.70] and
// rescaled to sum to 1 (see normalise).
//
// It is deterministic: rows are sorted by ID, every product is wrapped in
// float64() so no platform fuses it into an FMA (arm64 and amd64 agree), and
// the result is rounded to 1e-9.
func FitWeights(rated []Session) (w Weights, n int) {
	var kept []Session
	for _, s := range rated {
		if s.Rating >= 1 && s.Rating <= 5 && s.EndMS > 0 && s.Switches >= 0 {
			kept = append(kept, s)
		}
	}
	slices.SortFunc(kept, func(a, b Session) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	n = len(kept)
	if n < MinRated {
		return DefaultQualityWeights(), n
	}
	xs := make([][4]float64, n)
	ys := make([]float64, n)
	for i, s := range kept {
		xs[i], _ = Components(s)
		ys[i] = float64(s.Rating-1) / 4
	}
	v := normalise(fitRaw(xs, ys))
	return Weights{Share: v[0], Longest: v[1], Calm: v[2], Steady: v[3]}, n
}

// fitRaw solves the normal equations (XᵀX + λI) w = Xᵀy + λ w0 by Gaussian
// elimination with partial pivoting. The matrix is symmetric positive
// definite (every eigenvalue is at least λ) so it is never singular; with no
// rows the answer is exactly the defaults.
func fitRaw(xs [][4]float64, ys []float64) [4]float64 {
	d := DefaultQualityWeights()
	def := [4]float64{d.Share, d.Longest, d.Calm, d.Steady}
	var a [4][5]float64
	for i := 0; i < 4; i++ {
		for j := 0; j < 4; j++ {
			var s float64
			for r := range xs {
				s += float64(xs[r][i] * xs[r][j]) // float64() forbids FMA fusion
			}
			a[i][j] = s
		}
		a[i][i] += ridgeLambda
		var s float64
		for r := range xs {
			s += float64(xs[r][i] * ys[r])
		}
		a[i][4] = s + float64(ridgeLambda*def[i])
	}
	for c := 0; c < 4; c++ {
		p := c
		for r := c + 1; r < 4; r++ {
			if math.Abs(a[r][c]) > math.Abs(a[p][c]) {
				p = r
			}
		}
		a[c], a[p] = a[p], a[c]
		for r := c + 1; r < 4; r++ {
			f := a[r][c] / a[c][c]
			for k := c; k < 5; k++ {
				a[r][k] -= float64(f * a[c][k])
			}
		}
	}
	var w [4]float64
	for i := 3; i >= 0; i-- {
		s := a[i][4]
		for k := i + 1; k < 4; k++ {
			s -= float64(a[i][k] * w[k])
		}
		w[i] = s / a[i][i]
	}
	return w
}

// normalise clamps each weight to [0.05, 0.70] and rescales them to sum to
// 1. A weight that the rescaling pushes out of the bounds is pinned at the
// bound and the rest are rescaled again (at most four passes), so both the
// bounds and the sum hold. The result is rounded to 1e-9, with the sum kept
// within 1e-9 of 1.
func normalise(raw [4]float64) [4]float64 {
	var w [4]float64
	var pinned [4]bool
	for i, v := range raw {
		w[i] = min(max(v, minWeight), maxWeight)
	}
	for pass := 0; pass < 4; pass++ {
		var fixed, free float64
		for i := range w {
			if pinned[i] {
				fixed += w[i]
			} else {
				free += w[i]
			}
		}
		if free <= 0 {
			break
		}
		k := (1 - fixed) / free
		again := false
		for i := range w {
			if pinned[i] {
				continue
			}
			w[i] = float64(w[i] * k)
			if pass < 3 && (w[i] < minWeight || w[i] > maxWeight) {
				w[i] = min(max(w[i], minWeight), maxWeight)
				pinned[i] = true
				again = true
			}
		}
		if !again {
			break
		}
	}
	// snap to 1e-9 in integer units, then spread a rounding excess of more
	// than one unit so the sum stays within 1e-9 of 1.
	var u [4]int64
	var sum int64
	for i := range w {
		u[i] = int64(math.Round(float64(w[i] * snapUnit)))
		sum += u[i]
	}
	for diff := int64(snapUnit) - sum; diff > 1 || diff < -1; diff = int64(snapUnit) - sum {
		step := int64(1)
		if diff < 0 {
			step = -1
		}
		best := -1
		for i := range u {
			v := float64(u[i]+step) / snapUnit
			if v < minWeight || v > maxWeight {
				continue
			}
			if best < 0 || u[i] > u[best] {
				best = i
			}
		}
		if best < 0 {
			break
		}
		u[best] += step
		sum += step
	}
	for i := range u {
		w[i] = float64(u[i]) / snapUnit
	}
	return w
}
