package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/focus"
)

func iv(t *testing.T, from, to string) classify.Interval {
	return classify.Interval{From: apptest.At(t, from, 0), To: apptest.At(t, to, 0)}
}

func TestAwayAnswerSkipRelabel(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	d, err := a.Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	// The idle span 11:41-12:22 and the gap 12:25-14:50.
	lunch, gap := iv(t, "11:41", "12:22"), iv(t, "12:25", "14:50")
	if !reflect.DeepEqual(d.Away, []classify.Interval{lunch, gap}) {
		t.Fatalf("pending = %v", d.Away)
	}
	tracked, idle := d.TrackedMS, d.IdleMS

	if err := a.Answer(lunch, classify.Want{Category: "Communication"}); err != nil {
		t.Fatal(err)
	}
	d, _ = a.Day(apptest.Now)
	if !reflect.DeepEqual(d.Away, []classify.Interval{gap}) {
		t.Fatalf("pending after answer = %v", d.Away)
	}
	if d.TrackedMS != tracked+41*60_000 || d.IdleMS != idle-41*60_000 {
		t.Fatalf("tracked %d idle %d", d.TrackedMS, d.IdleMS)
	}
	if got := timeline(t, a); !strings.Contains(got, "11:41-12:22 Away Communication") {
		t.Fatalf("timeline:\n%s", got)
	}
	var awayRow []int64
	for _, r := range d.Timeline {
		if r.App == "Away" {
			awayRow = r.IDs
		}
	}
	if len(awayRow) != 1 || awayRow[0] >= 0 {
		t.Fatalf("away row ids = %v", awayRow)
	}

	// t/p on the away row relabels its away row.
	if err := a.Tag(awayRow, classify.Want{Project: "tock"}); err != nil {
		t.Fatal(err)
	}
	if got := timeline(t, a); !strings.Contains(got, "11:41-12:22 Away Communication") {
		t.Fatalf("project tag dropped the category:\n%s", got)
	}
	d, _ = a.Day(apptest.Now)
	for _, r := range d.Timeline {
		if r.App == "Away" && r.Label.Project != "tock" {
			t.Fatalf("away project = %+v", r.Label)
		}
	}

	// Skipping the gap: no more questions, nothing labelled.
	if err := a.Answer(gap, classify.Want{}); err != nil {
		t.Fatal(err)
	}
	d, _ = a.Day(apptest.Now)
	if len(d.Away) != 0 || strings.Contains(timeline(t, a), "12:25-14:50") {
		t.Fatalf("after skip: pending %v\n%s", d.Away, timeline(t, a))
	}
}

func TestAwayRespectsConfig(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.WriteConfig(t, "[away]\nmin = 45\nmax = 120\n")
	d, err := open(t, e).Day(apptest.Now)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Away) != 0 { // 41m is too short, 2h25 too long
		t.Fatalf("pending = %v", d.Away)
	}
}

func TestAwayDropsPastCache(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "x", Day: -1},
		apptest.Span{ID: 2, Start: "10:30", End: "11:00", Bundle: "term", App: "WezTerm", Title: "x", Day: -1})
	a := open(t, e)
	if _, err := a.Day(apptest.Now); err != nil { // fills the past cache
		t.Fatal(err)
	}
	before := a.past[dateKey(a.midnight(apptest.Now).AddDate(0, 0, -1))].ActiveMS
	from, to := apptest.At(t, "10:00", -1), apptest.At(t, "10:30", -1)
	if err := a.Answer(classify.Interval{From: from, To: to}, classify.Want{Category: "Study"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Day(apptest.Now); err != nil {
		t.Fatal(err)
	}
	if after := a.past[dateKey(a.midnight(apptest.Now).AddDate(0, 0, -1))].ActiveMS; after != before+30*60_000 {
		t.Fatalf("past active %d -> %d", before, after)
	}
}

func TestFocusTickAsksAboutAwayOnce(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Clock = time.UnixMilli(apptest.At(t, "14:51", 0)).In(apptest.Zone) // back at 14:50
	a := open(t, e)
	notes, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Title != focus.TitleAway || notes[0].Body != "Away 2h25 (12:25–14:50). What was it? Run tock away or open tock." {
		t.Fatalf("notes = %+v", notes)
	}
	if notes, err := open(t, e).FocusTick(false); err != nil || len(notes) != 0 {
		t.Fatalf("second tick (restarted) = %+v, %v", notes, err)
	}
}

func TestRecentAwayLabels(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	answer := func(from, to string, w classify.Want) {
		t.Helper()
		if err := a.Answer(iv(t, from, to), w); err != nil {
			t.Fatal(err)
		}
		e.Clock = e.Clock.Add(time.Minute) // later answers are newer
	}
	answer("01:00", "01:30", classify.Want{Category: "Communication"})
	answer("02:00", "02:30", classify.Want{Category: "Study", Project: "ATLAS"})
	answer("03:00", "03:30", classify.Want{}) // skipped: not a label
	answer("04:00", "04:30", classify.Want{Category: "Communication"})
	answer("05:00", "05:30", classify.Want{Category: "Gone"}) // undeclared
	got, err := a.RecentAwayLabels(3)
	if err != nil {
		t.Fatal(err)
	}
	want := []classify.Want{{Category: "Communication"}, {Category: "Study", Project: "ATLAS"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recent = %+v", got)
	}
}
