package collect

import (
	"reflect"
	"testing"
)

func TestParseEvents(t *testing.T) {
	got, err := parseEvents("100\t200\tWork\tStandup\n300\t300\tWork\tzero\n400\t500\t\ttab\tin title\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []CalendarEvent{{StartMS: 100, EndMS: 200, Title: "Standup", Calendar: "Work"}, {StartMS: 400, EndMS: 500, Title: "tab\tin title"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v", got)
	}
	if got, err := parseEvents(""); err != nil || got != nil {
		t.Fatalf("empty = %v, %v", got, err)
	}
	for _, bad := range []string{"1\t2\tx\n", "a\t2\tx\ty\n"} {
		if _, err := parseEvents(bad); err == nil {
			t.Errorf("parseEvents(%q) = nil error", bad)
		}
	}
}

func TestParseEventsV2(t *testing.T) {
	got, err := parseEvents("v2\t100\t200\tWork\tRoom 4\thttps://zoom.us/j/1\tnotes here\tStand\tup\n" +
		"300\t400\tHome\tOld\n" +
		"v2\t500\t500\tWork\t\t\t\tzero\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []CalendarEvent{
		{StartMS: 100, EndMS: 200, Calendar: "Work", Location: "Room 4", URL: "https://zoom.us/j/1", Notes: "notes here", Title: "Stand\tup"},
		{StartMS: 300, EndMS: 400, Calendar: "Home", Title: "Old"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v", got)
	}
	for _, bad := range []string{"v2\t1\t2\tc\tl\tu\tn\n", "v2\ta\t2\tc\tl\tu\tn\tt\n"} {
		if _, err := parseEvents(bad); err == nil {
			t.Errorf("parseEvents(%q) = nil error", bad)
		}
	}
}

func TestCalendarStatus(t *testing.T) {
	for s, want := range map[int]string{0: "undetermined", 1: "denied", 2: "denied", 3: "granted", 4: "denied", -1: "error"} {
		if got := calendarStatus(s); got != want {
			t.Errorf("calendarStatus(%d) = %q, want %q", s, got, want)
		}
	}
}
