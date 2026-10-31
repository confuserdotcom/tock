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

func TestParseEventsV3Mine(t *testing.T) {
	got, err := parseEvents("v3\t100\t200\tWork\t1\tRoom 4\thttps://zoom.us/j/1\tnotes\tStand\tup\n" +
		"v3\t300\t400\tWork\t0\t\t\t\tInvite\n" +
		"v2\t500\t600\tWork\t\thttps://zoom.us/j/2\t\tOld shim\n" +
		"700\t800\tHome\tOlder\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []CalendarEvent{
		{StartMS: 100, EndMS: 200, Calendar: "Work", Mine: true, Location: "Room 4", URL: "https://zoom.us/j/1", Notes: "notes", Title: "Stand\tup"},
		{StartMS: 300, EndMS: 400, Calendar: "Work", Title: "Invite"},
		{StartMS: 500, EndMS: 600, Calendar: "Work", URL: "https://zoom.us/j/2", Title: "Old shim"}, // a shim that does not say: not mine
		{StartMS: 700, EndMS: 800, Calendar: "Home", Title: "Older"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v\nwant     %+v", got, want)
	}
	for _, bad := range []string{"v3\t1\t2\tc\t1\tl\tu\tn\n", "v3\t1\t2\tc\tmaybe\tl\tu\tn\tt\n"} {
		if _, err := parseEvents(bad); err == nil {
			t.Errorf("parseEvents(%q) = nil error", bad)
		}
	}
}
