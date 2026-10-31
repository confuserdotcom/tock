package focus

import (
	"math"
	"testing"
)

// vecX and vecR are the 12 rated sessions of the research test vector.
var vecX = [][4]float64{
	{0.9, 0.8, 1, 0.9}, {0.8, 0.6, 0.5, 0.8}, {0.7, 0.9, 0.33, 0.5}, {0.5, 0.4, 0.25, 0.5},
	{0.95, 0.9, 1, 1}, {0.6, 0.5, 0.5, 0.7}, {0.3, 0.2, 0.2, 0.4}, {0.85, 0.7, 0.33, 0.9},
	{0.4, 0.3, 0.5, 0.6}, {0.75, 0.75, 1, 0.8}, {0.55, 0.45, 0.25, 0.3}, {0.65, 0.55, 0.5, 0.6},
}

var vecR = []int{5, 4, 3, 2, 5, 3, 1, 3, 2, 5, 2, 3}

func ratingTargets(r []int) []float64 {
	ys := make([]float64, len(r))
	for i, v := range r {
		ys[i] = float64(v-1) / 4
	}
	return ys
}

func near(t *testing.T, name string, got, want [4]float64, tol float64) {
	t.Helper()
	for i := range got {
		if math.Abs(got[i]-want[i]) > tol {
			t.Errorf("%s = %v, want %v (tol %g)", name, got, want, tol)
			return
		}
	}
}

func TestFitWeightsVector(t *testing.T) {
	raw := fitRaw(vecX, ratingTargets(vecR))
	near(t, "raw", raw, [4]float64{0.397360126, 0.219609930, 0.233026272, 0.102831703}, 1e-9)
	got := normalise(raw)
	want := [4]float64{0.417032364, 0.230482231, 0.244562780, 0.107922626}
	if got != want {
		t.Errorf("final = %v, want %v", got, want)
	}
}

func TestFitWeightsEmptyAndExact(t *testing.T) {
	def := [4]float64{0.45, 0.25, 0.15, 0.15}
	if got := fitRaw(nil, nil); got != def {
		t.Errorf("fitRaw(nil) = %v, want %v", got, def)
	}
	ys := make([]float64, len(vecX))
	for i, x := range vecX {
		for j := range x {
			ys[i] += def[j] * x[j]
		}
	}
	near(t, "exact", fitRaw(vecX, ys), def, 1e-12)
}

func TestFitWeightsShareTarget(t *testing.T) {
	ys := make([]float64, len(vecX))
	for i, x := range vecX {
		ys[i] = x[0]
	}
	near(t, "share", normalise(fitRaw(vecX, ys)), [4]float64{0.463402, 0.256612, 0.119770, 0.160216}, 1e-6)
}

// rated builds a finished 50 min manual session whose parts follow i, rated
// by pattern r.
func rated(id int64, i, r int) Session {
	const length = 50 * 60_000
	deep := int64(i%10+1) * length / 11
	return Session{ID: id, Kind: KindManual, StartMS: 1000, EndMS: 1000 + length, DeepMS: deep,
		LongestMS: deep / 2, Interruptions: i % 4, Switches: i % 6, Rating: r}
}

func ratedSet(n int) []Session {
	out := make([]Session, n)
	for i := range out {
		out[i] = rated(int64(i+1), i, 1+(i*7)%5)
	}
	return out
}

func TestFitWeightsGate(t *testing.T) {
	def := DefaultQualityWeights()
	nine := ratedSet(9)
	if w, n := FitWeights(nine); w != def || n != 9 {
		t.Fatalf("9 rated = %v, %d; want defaults, 9", w, n)
	}
	w, n := FitWeights(ratedSet(10))
	if n != 10 || w == def {
		t.Fatalf("10 rated = %v, %d; want fitted, 10", w, n)
	}
}

func TestFitWeightsLegacyExcluded(t *testing.T) {
	base := ratedSet(10)
	want, _ := FitWeights(base)
	extra := append([]Session(nil), base...)
	legacy := rated(100, 3, 5)
	legacy.Switches = -1
	unrated := rated(101, 4, 0)
	running := rated(102, 5, 3)
	running.EndMS = 0
	extra = append(extra, legacy, unrated, running)
	got, n := FitWeights(extra)
	if n != 10 || got != want {
		t.Fatalf("with ineligible rows = %v, %d; want %v, 10", got, n, want)
	}
	// nine eligible plus a legacy row stays below the gate.
	if w, n := FitWeights(append(ratedSet(9), legacy)); n != 9 || w != DefaultQualityWeights() {
		t.Fatalf("9 + legacy = %v, %d", w, n)
	}
}

func checkBounds(t *testing.T, w [4]float64) {
	t.Helper()
	var sum float64
	for _, v := range w {
		if v < 0.05 || v > 0.70 {
			t.Errorf("weight %v out of bounds in %v", v, w)
		}
		sum += v
	}
	if math.Abs(sum-1) > 1e-9+1e-12 {
		t.Errorf("sum %v of %v", sum, w)
	}
}

func TestFitWeightsBounds(t *testing.T) {
	for _, raw := range [][4]float64{
		{-0.3, 0.9, 0.3, 0.2},
		{0.95, 0.1, 0.1, 0.1},
		{1.4, -0.4, -0.2, 0.05},
		{0.45, 0.25, 0.15, 0.15},
		{0.9, 0.9, 0.9, 0.9},
		{0, 0, 0, 0},
	} {
		checkBounds(t, normalise(raw))
	}
	// end to end: the rating follows only the longest part
	var ss []Session
	for i := 0; i < 12; i++ {
		s := rated(int64(i+1), i, 1)
		s.DeepMS = 25 * 60_000
		s.LongestMS = int64(i) * 250_000
		s.Rating = 1 + i*4/11
		ss = append(ss, s)
	}
	w, _ := FitWeights(ss)
	checkBounds(t, [4]float64{w.Share, w.Longest, w.Calm, w.Steady})
}

func TestFitWeightsDeterministic(t *testing.T) {
	ss := ratedSet(14)
	w1, n1 := FitWeights(ss)
	w2, n2 := FitWeights(ss)
	if w1 != w2 || n1 != n2 {
		t.Fatalf("two runs differ: %v %v", w1, w2)
	}
	shuf := append([]Session(nil), ss...)
	for i, j := 0, len(shuf)-1; i < j; i, j = i+1, j-1 {
		shuf[i], shuf[j] = shuf[j], shuf[i]
	}
	shuf[0], shuf[5] = shuf[5], shuf[0]
	if w3, _ := FitWeights(shuf); w3 != w1 {
		t.Fatalf("shuffled input differs: %v vs %v", w3, w1)
	}
}
