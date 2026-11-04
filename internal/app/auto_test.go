package app

import (
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/classify"
)

func TestAutoAccept(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "09:00", End: "09:20", Bundle: "safari", App: "Safari", URL: "chess.com/play"},
		apptest.Span{ID: 2, Start: "09:20", End: "09:25", Bundle: "arc", App: "Arc", URL: "chess.com/analysis"},
	)
	a := open(t, e)
	if err := a.Tag([]int64{1}, classify.Want{Category: "Entertainment"}); err != nil {
		t.Fatal(err)
	}
	got, err := a.AutoAccept(7)
	if err != nil || len(got) != 1 || got[0].Key != "Arc · chess.com" || got[0].Category != "Entertainment" ||
		got[0].Reason != classify.GuessSite || got[0].MS != 5*60_000 || got[0].Rule == "" {
		t.Fatalf("AutoAccept = %+v, %v", got, err)
	}
	if items, _ := a.Inbox(7); len(items) != 0 {
		t.Fatalf("inbox after auto = %+v", items)
	}
	if again, err := a.AutoAccept(7); err != nil || len(again) != 0 {
		t.Fatalf("second AutoAccept = %+v, %v", again, err)
	}
	if xs, err := a.AutoLabels(0); err != nil || len(xs) != 1 {
		t.Fatalf("AutoLabels = %+v, %v", xs, err)
	}
}

func TestAutoAcceptEveryThrottles(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	if err := a.AutoAcceptEvery(7); err != nil || a.autoAt == 0 {
		t.Fatalf("first run: autoAt %d, %v", a.autoAt, err)
	}
	first := a.autoAt
	e.Clock = e.Clock.Add(9 * time.Minute)
	_ = a.AutoAcceptEvery(7)
	if a.autoAt != first {
		t.Fatal("ran again within 10 minutes")
	}
	e.Clock = e.Clock.Add(2 * time.Minute)
	_ = a.AutoAcceptEvery(7)
	if a.autoAt == first {
		t.Fatal("did not run after 10 minutes")
	}
}
