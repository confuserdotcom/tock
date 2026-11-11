package app

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"tock/internal/apptest"
)

// G5, spec edge case: with Focus off no session starts, a running one ends
// on the next tick as if stopped, and trackd's guard asks for nothing.
func TestFocusOffSessions(t *testing.T) {
	e := distracted(t)
	a := open(t, e)
	s, err := a.StartSession(50)
	if err != nil {
		t.Fatal(err)
	}
	e.WriteConfig(t, "[focus]\nenabled = false\n")
	e.Clock = e.Clock.Add(5 * time.Minute)
	if _, err := a.StartSession(25); !errors.Is(err, ErrFocusOff) {
		t.Fatalf("start with focus off: %v", err)
	}
	_, g, err := a.GuardTick(false)
	if err != nil {
		t.Fatal(err)
	}
	if g.Start != nil || g.Block != nil {
		t.Fatalf("guard %+v", g)
	}
	f, err := a.Focus(e.Clock)
	if err != nil || f.Open != nil {
		t.Fatalf("session still open: %+v %v", f.Open, err)
	}
	if len(f.Today) == 0 || f.Today[0].ID != s.ID || f.Today[0].EndMS != e.Clock.UnixMilli() {
		t.Fatalf("session not ended now: %+v", f.Today)
	}
}

// AC11: groups change no span and no label.
func TestGroupsKeepLabels(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	a := open(t, e)
	on, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	e.WriteConfig(t, "[focus]\nenabled = false\n\n[routine]\nenabled = false\n\n[billing]\nenabled = false\n")
	a = reopen(t, a, e)
	off, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(on.Timeline, off.Timeline) || !reflect.DeepEqual(on.Categories, off.Categories) || !reflect.DeepEqual(on.Projects, off.Projects) {
		t.Fatal("groups off changed the day's spans or labels")
	}
}
