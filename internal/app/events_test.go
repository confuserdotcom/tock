package app

import (
	"reflect"
	"strings"
	"testing"

	"tock/internal/apptest"
	"tock/internal/classify"
)

const calendarRules = apptest.Rules + `
[[category]]
name = "Meetings"
kind = "shallow"

[calendar]
category = "Meetings"
`

func TestEventsLabelTimeAway(t *testing.T) {
	e := apptest.New(t, calendarRules)
	e.Seed(t, apptest.DaySpans()...)
	e.Event(t, "12:30", "13:30", "Standup")
	e.Event(t, "09:30", "10:00", "Planning") // during activity: changes nothing
	e.Event(t, "16:00", "17:00", "Later")    // after now (15:00)
	a := open(t, e)

	base := apptest.New(t, apptest.Rules)
	base.Seed(t, apptest.DaySpans()...)
	b, err := open(t, base).Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	if d.TrackedMS != b.TrackedMS+60*60_000 {
		t.Fatalf("tracked %d, want %d + 1h", d.TrackedMS, b.TrackedMS)
	}
	got := timeline(t, a)
	for _, want := range []string{"09:00-10:40 WezTerm Code", "12:30-13:30 Meeting Meetings"} {
		if !strings.Contains(got, want) {
			t.Errorf("timeline lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "16:00") {
		t.Fatalf("filled the future:\n%s", got)
	}
	// The meeting splits the gap: 12:25-12:30 is too short to ask about.
	if want := []classify.Interval{iv(t, "11:41", "12:22"), iv(t, "13:30", "14:50")}; !reflect.DeepEqual(d.Away, want) {
		t.Fatalf("pending = %v", d.Away)
	}
	if items, _ := a.Inbox(1); strings.Contains(keys(items), "Standup") {
		t.Fatal("a meeting reached the inbox")
	}

	// Tagging the meeting records an away row that keeps the calendar
	// category.
	var ids []int64
	for _, r := range d.Timeline {
		if r.App == "Meeting" {
			ids = r.IDs
		}
	}
	if err := a.Tag(ids, classify.Want{Project: "ATLAS"}); err != nil {
		t.Fatal(err)
	}
	d, _ = a.Day(apptest.Now)
	found := false
	for _, r := range d.Timeline {
		if r.App == "Away" && r.Label.Category == "Meetings" && r.Label.Project == "ATLAS" {
			found = true
		}
	}
	if !found {
		t.Fatalf("timeline after tag:\n%s", timeline(t, a))
	}

	// Without [calendar] category the events are not read.
	e.WriteRules(t, apptest.Rules)
	if _, err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := timeline(t, a); strings.Contains(got, " Meeting ") {
		t.Fatalf("calendar off:\n%s", got)
	}
}

func keys(items []classify.Item) string {
	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.Key + "\n")
	}
	return b.String()
}
