package focus

import (
	"testing"

	"tock/internal/span"
)

func TestLongestRunMS(t *testing.T) {
	min := func(n int64) int64 { return n * minute }
	cases := []struct {
		name string
		vs   []vw
		want int64
	}{
		{"no views", nil, 0},
		{"one view", []vw{{from: at(9, 0, 0), to: at(9, 50, 0)}}, min(50)},
		{"gap under 5 min is one run", []vw{
			{from: at(9, 0, 0), to: at(9, 20, 0)}, {from: at(9, 24, 0), to: at(9, 40, 0)}}, min(40)},
		{"gap of 5 min splits, longer wins", []vw{
			{from: at(9, 0, 0), to: at(9, 20, 0)}, {from: at(9, 25, 0), to: at(10, 0, 0)}}, min(35)},
		{"short idle inside stays one run", []vw{
			{from: at(9, 0, 0), to: at(9, 20, 0)}, {from: at(9, 20, 0), to: at(9, 23, 0), kind: "idle"},
			{from: at(9, 23, 0), to: at(9, 40, 0)}}, min(40)},
		{"idle of 6 min splits", []vw{
			{from: at(9, 0, 0), to: at(9, 30, 0)}, {from: at(9, 30, 0), to: at(9, 36, 0), kind: "idle"},
			{from: at(9, 36, 0), to: at(9, 50, 0)}}, min(30)},
		{"leading and trailing idle do not count", []vw{
			{from: at(9, 0, 0), to: at(9, 3, 0), kind: "idle"}, {from: at(9, 3, 0), to: at(9, 33, 0)},
			{from: at(9, 33, 0), to: at(9, 36, 0), kind: "idle"}}, min(30)},
		{"only idle", []vw{{from: at(9, 0, 0), to: at(9, 30, 0), kind: "idle"}}, 0},
		{"away bundle ends a run like idle", []vw{
			{from: at(9, 0, 0), to: at(9, 30, 0)}, {from: at(9, 30, 0), to: at(9, 40, 0), bundle: span.AwayBundleID},
			{from: at(9, 40, 0), to: at(9, 50, 0)}}, min(30)},
		{"zero-length and overlapping views are total", []vw{
			{from: at(9, 0, 0), to: at(9, 0, 0)}, {from: at(9, 0, 0), to: at(9, 30, 0)},
			{from: at(9, 10, 0), to: at(9, 20, 0)}}, min(30)},
	}
	for _, c := range cases {
		views, _ := world(c.vs...)
		if got := LongestRunMS(views); got != c.want {
			t.Errorf("%s: LongestRunMS = %d min, want %d", c.name, got/minute, c.want/minute)
		}
	}
}

// TestLongestRunMeasuresLikeRunStart: for a day whose last view is open at
// Now, the longest run is at least the run stepBreak is measuring.
func TestLongestRunMeasuresLikeRunStart(t *testing.T) {
	in := input(at(11, 0, 0),
		vw{from: at(8, 0, 0), to: at(8, 40, 0)},
		vw{from: at(9, 0, 0), to: at(9, 30, 0)},
		vw{from: at(9, 30, 0), to: at(9, 33, 0), kind: "idle"},
		vw{from: at(9, 33, 0), to: at(11, 0, 0)})
	from, ok := runStart(in)
	if !ok {
		t.Fatal("runStart not ok")
	}
	if got := LongestRunMS(in.Views); got < in.Now-from {
		t.Errorf("LongestRunMS %d < Now - runStart %d", got, in.Now-from)
	}
	if got, want := LongestRunMS(in.Views), at(11, 0, 0)-at(9, 0, 0); got != want {
		t.Errorf("LongestRunMS = %d min, want %d", got/minute, want/minute)
	}
}
