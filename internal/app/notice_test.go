package app

import (
	"reflect"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/focus"
)

func TestEnqueueAndTake(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "09:10", Bundle: "term", App: "WezTerm", Title: "x"})
	a := open(t, e)
	if err := a.Answer(classify.Interval{From: apptest.At(t, "10:00", 0), To: apptest.At(t, "10:30", 0)},
		classify.Want{Category: "Communication"}); err != nil {
		t.Fatal(err)
	}
	away := classify.Interval{From: apptest.At(t, "12:00", 0), To: apptest.At(t, "12:40", 0)}
	for _, ev := range []focus.Event{
		{Kind: focus.Notify, Topic: focus.TopicDone, Title: focus.TitleDone, Body: "done", Minutes: 50},
		{Kind: focus.Notify, Topic: focus.TopicAway, Title: focus.TitleAway, Body: "away", Away: away},
	} {
		if err := a.Enqueue(ev); err != nil {
			t.Fatal(err)
		}
	}
	ns, err := a.TakeNotices()
	if err != nil || len(ns) != 2 || ns[0].Topic != focus.TopicDone || ns[1].Body != "away" {
		t.Fatalf("TakeNotices = %+v, %v", ns, err)
	}
	if _, d, err := a.Notice(ns[0].ID); err != nil || d.Minutes != 50 {
		t.Fatalf("done data = %+v, %v", d, err)
	}
	_, d, err := a.Notice(ns[1].ID)
	want := NoticeData{From: away.From, To: away.To, Labels: []classify.Want{{Category: "Communication"}}}
	if err != nil || !reflect.DeepEqual(d, want) {
		t.Fatalf("away data = %+v, %v; want %+v", d, err, want)
	}
	if ns, _ := a.TakeNotices(); len(ns) != 0 {
		t.Fatalf("taken twice: %+v", ns)
	}

	// What the absence follows leads the buttons, once.
	for _, b := range []classify.Want{{Category: "Code", Project: "tock"}, {Category: "Communication"}} {
		if err := a.Enqueue(focus.Event{Kind: focus.Notify, Topic: focus.TopicAway, Title: focus.TitleAway, Away: away, Before: b}); err != nil {
			t.Fatal(err)
		}
		ns, _ := a.TakeNotices()
		_, d, _ := a.Notice(ns[0].ID)
		want := []classify.Want{b}
		if b.Category == "Code" {
			want = append(want, classify.Want{Category: "Communication"})
		}
		if !reflect.DeepEqual(d.Labels, want) {
			t.Fatalf("labels after %v = %+v", b, d.Labels)
		}
	}
}

func TestBarReady(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", End: "09:10", Bundle: "term", App: "WezTerm", Title: "x"})
	a := open(t, e)
	if a.BarReady() {
		t.Fatal("ready with no heartbeat")
	}
	if err := a.BarBeat(NotifyDenied); err != nil {
		t.Fatal(err)
	}
	if running, access, _ := a.BarState(); !running || access != NotifyDenied || a.BarReady() {
		t.Fatalf("denied: running %v access %q ready %v", running, access, a.BarReady())
	}
	if err := a.BarBeat(NotifyGranted); err != nil {
		t.Fatal(err)
	}
	if !a.BarReady() {
		t.Fatal("fresh and granted must be ready")
	}
	e.Clock = e.Clock.Add(BarFreshMS*time.Millisecond + time.Second)
	if running, _, _ := a.BarState(); running || a.BarReady() {
		t.Fatal("a stale heartbeat must not be ready")
	}
}
