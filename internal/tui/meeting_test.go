package tui

import (
	"strings"
	"testing"

	"tock/internal/apptest"
)

func TestMeetingRow(t *testing.T) {
	e := apptest.New(t, apptest.Rules+"\n[[category]]\nname = \"Meetings\"\nkind = \"shallow\"\n\n[calendar]\ncategory = \"Meetings\"\n")
	e.Seed(t, apptest.DaySpans()...)
	e.Event(t, "12:30", "13:30", "Standup")
	m := newModel(t, e)
	l := line(t, m, "12:30–13:30")
	if !strings.Contains(l, "Meeting") || !strings.Contains(l, "Standup") || !strings.Contains(l, "Meetings") {
		t.Fatalf("meeting row: %q", l)
	}
	for i := 0; i < 8 && !strings.Contains(line(t, m, "12:30–13:30"), "▶"); i++ {
		m = press(t, m, "k")
	}
	m = press(t, m, "T")
	mustContain(t, m, "meetings are learned by hand")
	m = press(t, m, "p")
	m = typeText(t, m, "atl")
	m = press(t, m, "enter")
	if l := line(t, m, "12:30–13:30"); !strings.Contains(l, "Meetings / ATLAS") {
		t.Fatalf("relabelled meeting: %q", l)
	}
}
