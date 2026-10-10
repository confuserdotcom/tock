package classify

import (
	"fmt"
	"strings"
	"testing"

	"tock/internal/span"
)

func show(vs []SpanView) string {
	var b []string
	for _, v := range vs {
		id := fmt.Sprint(v.ID)
		if e, ok := EditRowID(v.ID); ok {
			id = fmt.Sprintf("e%d", e)
		}
		b = append(b, fmt.Sprintf("%d-%d:%s:%s", v.Start, v.End, v.App, id))
	}
	return strings.Join(b, " ")
}

func editViews() []SpanView {
	return []SpanView{
		{ID: 1, Start: 0, End: 10, BundleID: "a", App: "A"},
		{ID: 2, Start: 10, End: 20, BundleID: span.IdleBundleID, App: "Idle"},
		{ID: 3, Start: 20, End: 30, BundleID: "b", App: "B"},
		{ID: 4, Start: 40, End: 50, BundleID: "c", App: "C"},
	}
}

func TestApplyEdits(t *testing.T) {
	cases := []struct {
		name  string
		edits []EditMark
		want  string
	}{
		{"none", nil, "0-10:A:1 10-20:Idle:2 20-30:B:3 40-50:C:4"},
		{"cut", []EditMark{{ID: 1, Kind: EditCut, Interval: Interval{5, 25}}},
			"0-5:A:1 25-30:B:3 40-50:C:4"},
		{"add replaces", []EditMark{{ID: 2, Kind: EditAdd, Note: "x", Interval: Interval{5, 45}}},
			"0-5:A:1 5-45:Entry:e2 45-50:C:4"},
		{"label keeps apps and fills idle and gaps", []EditMark{{ID: 3, Kind: EditLabel, Interval: Interval{5, 45}}},
			"0-5:A:1 5-10:A:e3 10-20:Entry:e3 20-30:B:e3 30-40:Entry:e3 40-45:C:e3 45-50:C:4"},
		{"later wins", []EditMark{{ID: 1, Kind: EditLabel, Interval: Interval{0, 30}}, {ID: 2, Kind: EditCut, Interval: Interval{0, 10}}},
			"10-20:Entry:e1 20-30:B:e1 40-50:C:4"},
		{"clipped to range", []EditMark{{ID: 5, Kind: EditAdd, Interval: Interval{45, 90}}},
			"0-10:A:1 10-20:Idle:2 20-30:B:3 40-45:C:4 45-60:Entry:e5"},
	}
	for _, c := range cases {
		if got := show(ApplyEdits(editViews(), c.edits, 0, 60)); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestEditIDsAreApart(t *testing.T) {
	id := -(EditIDBase + 7)
	if e, ok := EditRowID(id); !ok || e != 7 {
		t.Fatal("EditRowID")
	}
	if _, ok := RemoteRowID(id); ok {
		t.Fatal("an edit id reads as remote")
	}
	if _, ok := MeetingEventID(id); ok {
		t.Fatal("an edit id reads as a meeting")
	}
	if _, ok := EditRowID(-(RemoteIDBase + 7)); ok {
		t.Fatal("a remote id reads as an edit")
	}
}
