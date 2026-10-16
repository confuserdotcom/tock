package classify

import (
	"reflect"
	"testing"

	"tock/internal/span"
)

const mn = int64(60_000)

func av(id, start, end int64, bundle string) SpanView {
	return SpanView{ID: id, Start: start, End: end, BundleID: bundle, App: bundle}
}

func idleV(id, start, end int64) SpanView {
	return SpanView{ID: id, Start: start, End: end, BundleID: span.IdleBundleID, App: span.IdleApp}
}

// windows gives zathura a 10 min window and everything else 5.
func windows(v SpanView) int64 {
	if v.BundleID == "org.pwmt.zathura" {
		return 10 * mn
	}
	return 5 * mn
}

func TestMarkAFK(t *testing.T) {
	z := "org.pwmt.zathura"
	cases := []struct {
		name  string
		in    []SpanView
		stale int64
		want  []SpanView
	}{
		{"short idle after reading is reading",
			[]SpanView{av(1, 0, 10*mn, z), idleV(2, 10*mn, 17*mn), av(3, 17*mn, 20*mn, z)}, 0,
			[]SpanView{av(1, 0, 17*mn, z), av(3, 17*mn, 20*mn, z)}},
		{"long idle stays idle",
			[]SpanView{av(1, 0, 10*mn, z), idleV(2, 10*mn, 25*mn)}, 0,
			[]SpanView{av(1, 0, 10*mn, z), idleV(2, 10*mn, 25*mn)}},
		{"exactly the window is kept",
			[]SpanView{av(1, 0, 10*mn, z), idleV(2, 10*mn, 20*mn)}, 0,
			[]SpanView{av(1, 0, 20*mn, z)}},
		{"code window is the default",
			[]SpanView{av(1, 0, 10*mn, "wez"), idleV(2, 10*mn, 16*mn)}, 0,
			[]SpanView{av(1, 0, 10*mn, "wez"), idleV(2, 10*mn, 16*mn)}},
		{"idle not right after is untouched",
			[]SpanView{av(1, 0, 10*mn, z), idleV(2, 11*mn, 13*mn)}, 0,
			[]SpanView{av(1, 0, 10*mn, z), idleV(2, 11*mn, 13*mn)}},
		{"leading idle is untouched",
			[]SpanView{idleV(2, 0, 3*mn), av(1, 3*mn, 10*mn, z)}, 0,
			[]SpanView{idleV(2, 0, 3*mn), av(1, 3*mn, 10*mn, z)}},
		{"stale: long window keeps its window, rest idle",
			[]SpanView{av(1, 0, 393*mn, z)}, 20 * mn,
			[]SpanView{av(1, 0, 10*mn, z), idleV(1, 10*mn, 393*mn)}},
		{"stale: under the limit stays",
			[]SpanView{av(1, 0, 19*mn, z)}, 20 * mn,
			[]SpanView{av(1, 0, 19*mn, z)}},
		{"stale off: long window stays",
			[]SpanView{av(1, 0, 393*mn, z)}, 0,
			[]SpanView{av(1, 0, 393*mn, z)}},
		{"remote views are untouched",
			[]SpanView{av(-(RemoteIDBase + 4), 0, 10*mn, z), idleV(2, 10*mn, 12*mn)}, 20 * mn,
			[]SpanView{av(-(RemoteIDBase + 4), 0, 10*mn, z), idleV(2, 10*mn, 12*mn)}},
	}
	for _, c := range cases {
		got := MarkAFK(c.in, AFK{Window: windows, StaleMS: c.stale})
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}

func TestMarkAFKDoesNotChangeInput(t *testing.T) {
	in := []SpanView{av(1, 0, 10*mn, "org.pwmt.zathura"), idleV(2, 10*mn, 12*mn)}
	MarkAFK(in, AFK{Window: windows})
	if in[0].End != 10*mn || len(in) != 2 {
		t.Fatalf("input changed: %+v", in)
	}
}

func TestMarkAFKStaleWindowLongerThanView(t *testing.T) {
	in := []SpanView{av(1, 0, 25*mn, "x")}
	got := MarkAFK(in, AFK{Window: func(SpanView) int64 { return 30 * mn }, StaleMS: 20 * mn})
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("got %+v", got)
	}
}
