package classify

import (
	"fmt"
	"strings"
	"testing"

	"tock/internal/span"
)

// meetPieces is pieces with meeting ids shown as m<event id> and the title
// after the bundle.
func meetPieces(vs []SpanView) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		id := fmt.Sprint(v.ID)
		if e, ok := MeetingEventID(v.ID); ok {
			id = fmt.Sprintf("m%d", e)
		}
		parts[i] = fmt.Sprintf("%s:%d-%d %s", id, v.Start, v.End, v.BundleID)
		if IsMeeting(v) {
			parts[i] += " " + *v.Title
		}
	}
	return strings.Join(parts, ", ")
}

func TestMarkEvents(t *testing.T) {
	views := []SpanView{
		{ID: 1, Start: 0, End: 10, BundleID: "a"},
		{ID: 2, Start: 10, End: 25, BundleID: idleB},
		{ID: 3, Start: 40, End: 50, BundleID: "a"}, // activity inside an event stays
		{ID: 4, Start: 70, End: 80, BundleID: "b"},
	}
	evs := []EventMark{
		{ID: 9, Title: "late", Interval: Interval{30, 75}}, // overlaps 7: 7 started first and wins until 35
		{ID: 7, Title: "standup", Interval: Interval{5, 35}},
	}
	got := MarkEvents(views, evs, 0, 100)
	want := "1:0-10 a, m7:10-35 tock.meeting standup, m9:35-40 tock.meeting late, 3:40-50 a, " +
		"m9:50-70 tock.meeting late, 4:70-80 b"
	if meetPieces(got) != want {
		t.Fatalf("got  %s\nwant %s", meetPieces(got), want)
	}
	if got[1].App != span.MeetingApp || !IsMeeting(got[1]) || IsAway(got[1]) {
		t.Fatalf("meeting view = %+v", got[1])
	}
	// to (now) clips the fill: nothing after it.
	got = MarkEvents([]SpanView{{ID: 1, Start: 0, End: 10, BundleID: "a"}}, []EventMark{{ID: 2, Title: "x", Interval: Interval{5, 60}}}, 0, 30)
	if want := "1:0-10 a, m2:10-30 tock.meeting x"; meetPieces(got) != want {
		t.Fatalf("clipped: %s", meetPieces(got))
	}
	if got := MarkEvents(views, nil, 0, 100); meetPieces(got) != meetPieces(views) {
		t.Fatal("no events must change nothing")
	}
	if _, ok := MeetingEventID(-5); ok {
		t.Fatal("an away id is not a meeting id")
	}
	if id, ok := MeetingEventID(-(MeetingIDBase + 3)); !ok || id != 3 {
		t.Fatalf("MeetingEventID = %d %v", id, ok)
	}
}

func TestAwayWinsOverEvent(t *testing.T) {
	views := []SpanView{{ID: 1, Start: 0, End: 10, BundleID: "a"}, {ID: 2, Start: 40, End: 50, BundleID: "a"}}
	vs := MarkAway(views, []AwayMark{{4, Interval{10, 25}}}, 0, 100)
	vs = MarkEvents(vs, []EventMark{{ID: 1, Title: "m", Interval: Interval{10, 40}}}, 0, 100)
	if want := "1:0-10 a, -4:10-25 tock.away, m1:25-40 tock.meeting m, 2:40-50 a"; meetPieces(vs) != want {
		t.Fatalf("got %s", meetPieces(vs))
	}
	// A meeting is activity: no absence left to ask about.
	if abs := Absences(vs, 5, 100); abs != nil {
		t.Fatalf("absences = %v", abs)
	}
}

func TestCalendarLabels(t *testing.T) {
	rules := `
[[category]]
name = "Meetings"
kind = "shallow"

[[category]]
name = "Code"
kind = "deep"

[[project]]
name = "ATLAS"

[calendar]
category = "meetings"

[[rule]]
bundle = ["tock.meeting"]
title = '^ATLAS'
project = "ATLAS"

[[rule]]
bundle = ["tock.meeting"]
title = '^pairing'
category = "Code"
`
	rs, err := Parse([]byte(rules), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rs.CalendarCategory != "Meetings" {
		t.Fatalf("CalendarCategory = %q", rs.CalendarCategory)
	}
	meet := func(title string) SpanView {
		return SpanView{ID: -(MeetingIDBase + 1), BundleID: span.MeetingBundleID, App: span.MeetingApp, Title: &title}
	}
	l := rs.Label(meet("ATLAS sync"), nil)
	if l.Category != "Meetings" || l.Kind != "shallow" || l.CategorySource != SourceCalendar || l.Project != "ATLAS" || l.ProjectSource != SourceRule {
		t.Fatalf("ATLAS label = %+v", l)
	}
	if l := rs.Label(meet("pairing"), nil); l.Category != "Code" || l.CategorySource != SourceRule {
		t.Fatalf("a rule category wins: %+v", l)
	}
	if l := rs.Label(meet("x"), &Override{Category: "Code"}); l.Category != "Code" || l.CategorySource != SourceManual {
		t.Fatalf("an override wins: %+v", l)
	}
	// Meeting views never enter the inbox.
	if items := Inbox([]SpanView{meet("y")}, []Label{{}}, nil); len(items) != 0 {
		t.Fatalf("inbox = %+v", items)
	}
	if _, err := Parse([]byte(rules+"\n[calendar]\n"), nil); err == nil {
		t.Fatal("a duplicate [calendar] table must fail")
	}
	if _, err := Parse([]byte(strings.Replace(rules, `category = "meetings"`, `category = "Meet"`, 1)), nil); err == nil ||
		!strings.Contains(err.Error(), `[calendar]: unknown category "Meet"`) {
		t.Fatalf("unknown category err = %v", err)
	}
}
