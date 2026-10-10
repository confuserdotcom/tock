package classify

import (
	"reflect"
	"testing"

	"tock/internal/span"
)

const idleB = span.IdleBundleID

func TestAbsences(t *testing.T) {
	for _, c := range []struct {
		name  string
		views []SpanView
		want  []Interval
	}{
		{"gap", []SpanView{{Start: 0, End: 10, BundleID: "a"}, {Start: 30, End: 40, BundleID: "a"}}, []Interval{{10, 30}}},
		{"idle", []SpanView{{Start: 0, End: 10, BundleID: "a"}, {Start: 10, End: 30, BundleID: idleB}, {Start: 30, End: 40, BundleID: "b"}},
			[]Interval{{10, 30}}},
		{"idle then gap merge", []SpanView{{Start: 0, End: 10, BundleID: "a"}, {Start: 10, End: 20, BundleID: idleB}, {Start: 35, End: 40, BundleID: "b"}},
			[]Interval{{10, 35}}},
		{"too short", []SpanView{{Start: 0, End: 10, BundleID: "a"}, {Start: 14, End: 40, BundleID: "a"}}, nil},
		{"too long", []SpanView{{Start: 0, End: 10, BundleID: "a"}, {Start: 111, End: 120, BundleID: "a"}}, nil},
		{"bounds inclusive", []SpanView{{Start: 0, End: 10, BundleID: "a"}, {Start: 15, End: 20, BundleID: "a"}, {Start: 120, End: 130, BundleID: "a"}},
			[]Interval{{10, 15}, {20, 120}}},
		{"no return yet", []SpanView{{Start: 0, End: 10, BundleID: "a"}, {Start: 10, End: 60, BundleID: idleB}}, nil},
		{"before the first activity", []SpanView{{Start: 0, End: 50, BundleID: idleB}, {Start: 50, End: 60, BundleID: "a"}}, nil},
		{"a call splits it", []SpanView{{Start: 0, End: 10, BundleID: "a"}, {Start: 10, End: 20, BundleID: idleB},
			{Start: 20, End: 30, BundleID: span.CallBundleID}, {Start: 30, End: 45, BundleID: idleB}, {Start: 45, End: 50, BundleID: "a"}},
			[]Interval{{10, 20}, {30, 45}}},
	} {
		if got := Absences(c.views, 5, 100); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMarkAway(t *testing.T) {
	views := []SpanView{
		{ID: 1, Start: 0, End: 10, BundleID: "a"},
		{ID: 2, Start: 10, End: 25, BundleID: idleB},
		{ID: 3, Start: 40, End: 50, BundleID: "a"},
		{ID: 4, Start: 50, End: 70, BundleID: idleB},
		{ID: 5, Start: 90, End: 100, BundleID: "b"},
	}
	// Mark 7 covers an idle span and the gap after it; mark 8 only part of
	// span 4's idle time and the gap after it. Each becomes one away view.
	got := MarkAway(views, []AwayMark{{7, Interval{10, 40}}, {8, Interval{60, 90}}}, 0, 100)
	if want := "1:0-10 a, -7:10-40 tock.away, 3:40-50 a, 4:50-60 tock.idle, -8:60-90 tock.away, 5:90-100 b"; pieces(got) != want {
		t.Fatalf("got  %s\nwant %s", pieces(got), want)
	}
	if got[1].App != span.AwayApp || got[1].Title != nil || !IsAway(got[1]) {
		t.Fatalf("away view = %+v", got[1])
	}
	// The window clips the filled gaps.
	got = MarkAway([]SpanView{{ID: 1, Start: 50, End: 60, BundleID: "a"}}, []AwayMark{{3, Interval{0, 50}}}, 20, 100)
	if want := "-3:20-50 tock.away, 1:50-60 a"; pieces(got) != want {
		t.Fatalf("clipped: %s", pieces(got))
	}
	if got := MarkAway(views, nil, 0, 100); !reflect.DeepEqual(got, views) {
		t.Fatal("no marks must keep views")
	}
}
