package app

import (
	"errors"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/store"
)

func TestSnoozeDistract(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	e.Clock = apptest.Now
	if _, err := a.SnoozeDistract(0, 10*time.Minute); !errors.Is(err, store.ErrNoSession) {
		t.Fatalf("no session: %v, want ErrNoSession", err)
	}
	if v, err := a.distractSnooze(); err != nil || v != 0 {
		t.Fatalf("no session wrote %d, %v", v, err)
	}

	s := startAt(t, e, a, "14:50", 25)
	e.Clock = apptest.Now
	if _, err := a.SnoozeDistract(s.ID+1, 10*time.Minute); !errors.Is(err, store.ErrNoSession) {
		t.Fatalf("other session: %v, want ErrNoSession", err)
	}
	until, err := a.SnoozeDistract(s.ID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if want := apptest.Now.Add(10 * time.Minute); !until.Equal(want) || until.Location() != apptest.Zone {
		t.Fatalf("until = %v (%v), want %v in %v", until, until.Location(), want, apptest.Zone)
	}
	if was, err := a.ResumeDistract(); err != nil || !was {
		t.Fatalf("first resume = %v, %v; want true", was, err)
	}
	if was, err := a.ResumeDistract(); err != nil || was {
		t.Fatalf("second resume = %v, %v; want false", was, err)
	}
	// Session 0 means whatever runs.
	if _, err := a.SnoozeDistract(0, time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestDistractSnoozeSurvivesTick(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	s := startAt(t, e, a, "14:50", 25)
	e.Clock = apptest.Now
	until, err := a.SnoozeDistract(s.ID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	trackd := open(t, e) // a second process on the same files
	for _, hhmm := range []string{"15:00", "15:03"} {
		e.Clock = time.UnixMilli(apptest.At(t, hhmm, 0)).In(apptest.Zone)
		if notes, err := trackd.FocusTick(false); err != nil || len(notes) != 0 {
			t.Fatalf("%s: snoozed tick = %+v, %v", hhmm, notes, err)
		}
	}
	got, err := a.distractSnooze()
	if err != nil || got != until.UnixMilli() {
		t.Fatalf("snooze after ticks = %d, %v; want %d", got, err, until.UnixMilli())
	}
}

func TestFocusCarriesSnooze(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	e.Clock = apptest.Now
	f, err := a.Focus(e.Clock)
	if err != nil || f.SnoozeMin != 10 || f.SnoozeUntilMS != 0 {
		t.Fatalf("no session: SnoozeMin %d, SnoozeUntilMS %d, %v; want 10, 0", f.SnoozeMin, f.SnoozeUntilMS, err)
	}
	// A snooze left behind with no session running is not reported.
	if err := a.st.SetState(store.KeyFocusDistractSnooze, "99999999999999", 1); err != nil {
		t.Fatal(err)
	}
	if f, err = a.Focus(e.Clock); err != nil || f.SnoozeUntilMS != 0 {
		t.Fatalf("no session, stale key: SnoozeUntilMS %d, %v; want 0", f.SnoozeUntilMS, err)
	}

	s := startAt(t, e, a, "14:50", 25)
	e.Clock = apptest.Now
	if f, err = a.Focus(e.Clock); err != nil || f.SnoozeUntilMS != 0 {
		t.Fatalf("not snoozed: SnoozeUntilMS %d, %v; want 0", f.SnoozeUntilMS, err)
	}
	until, err := a.SnoozeDistract(s.ID, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if f, err = a.Focus(e.Clock); err != nil || f.SnoozeUntilMS != until.UnixMilli() || f.SnoozeMin != 10 {
		t.Fatalf("snoozed: SnoozeUntilMS %d, SnoozeMin %d, %v; want %d, 10", f.SnoozeUntilMS, f.SnoozeMin, err, until.UnixMilli())
	}
	e.Clock = until.Add(time.Minute)
	if f, err = a.Focus(e.Clock); err != nil || f.SnoozeUntilMS != 0 {
		t.Fatalf("expired: SnoozeUntilMS %d, %v; want 0", f.SnoozeUntilMS, err)
	}
}
