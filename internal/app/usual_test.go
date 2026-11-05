package app

import (
	"testing"
	"time"

	"tock/internal/apptest"
)

func TestUsualIsTheMedianOfTheSameWeekday(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	code := func(id int64, day int, end string) apptest.Span {
		return apptest.Span{ID: id, Start: "09:00", End: end, Bundle: "term", App: "WezTerm", Title: "tock │ a", Day: day}
	}
	e.Seed(t,
		code(201, -7, "10:00"),  // 1h
		code(202, -14, "11:00"), // 2h
		code(203, -21, "12:00"), // 3h
		code(204, -1, "17:00"),  // another weekday: left out
		// -28 has nothing: a day off is not part of the usual day.
	)
	a := open(t, e)
	u, err := a.Usual(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if u.Days != 3 {
		t.Fatalf("days %d, want 3", u.Days)
	}
	if u.TrackedMS != (2 * time.Hour).Milliseconds() {
		t.Errorf("tracked %v, want the median 2h", time.Duration(u.TrackedMS)*time.Millisecond)
	}
	if u.LongestMS != (2 * time.Hour).Milliseconds() {
		t.Errorf("longest %v, want 2h", time.Duration(u.LongestMS)*time.Millisecond)
	}
	if u.Score <= 0 {
		t.Errorf("score %d, want the median score", u.Score)
	}
}

func TestUsualNeedsTwoDays(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Seed(t, apptest.Span{ID: 201, Start: "09:00", End: "10:00", Bundle: "term", App: "WezTerm", Title: "tock │ a", Day: -7})
	a := open(t, e)
	u, err := a.Usual(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if u.Days != 0 {
		t.Fatalf("one past day gave a usual day: %+v", u)
	}
}
