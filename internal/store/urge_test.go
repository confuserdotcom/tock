package store

import (
	"errors"
	"testing"
)

func TestAddUrge(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	a, err := s.StartSession(1000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	a.EndMS = 3000
	if ok, err := s.EndSession(a); err != nil || !ok {
		t.Fatalf("EndSession = %v, %v", ok, err)
	}
	b, err := s.StartSession(4000, 5000)
	if err != nil {
		t.Fatal(err)
	}
	for _, given := range []bool{false, false, true} {
		if err := s.AddUrge(b.ID, given); err != nil {
			t.Fatalf("AddUrge(%d, %v): %v", b.ID, given, err)
		}
	}
	got, err := s.OpenSession()
	if err != nil || got == nil {
		t.Fatalf("OpenSession = %v, %v", got, err)
	}
	if got.UrgesResisted != 2 || got.UrgesGiven != 1 {
		t.Fatalf("urges = %d resisted, %d given, want 2, 1", got.UrgesResisted, got.UrgesGiven)
	}
	other, err := s.SessionByID(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if other.UrgesResisted != 0 || other.UrgesGiven != 0 {
		t.Fatalf("other session urges = %d/%d, want 0/0", other.UrgesResisted, other.UrgesGiven)
	}
	if err := s.AddUrge(9999, false); !errors.Is(err, ErrSessionGone) {
		t.Fatalf("AddUrge unknown id err = %v, want ErrSessionGone", err)
	}
}

func TestAddUrgeEndedSession(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	a, err := s.StartSession(1000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	a.EndMS = 3000
	if ok, err := s.EndSession(a); err != nil || !ok {
		t.Fatalf("EndSession = %v, %v", ok, err)
	}
	if err := s.AddUrge(a.ID, true); err != nil {
		t.Fatalf("AddUrge on ended session: %v", err)
	}
	got, err := s.SessionByID(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.UrgesGiven != 1 || got.UrgesResisted != 0 {
		t.Fatalf("urges = %d resisted, %d given, want 0, 1", got.UrgesResisted, got.UrgesGiven)
	}
}

// TestAddUrgeSurvivesTrackd: trackd updates and ends the session from a value
// read before the answer; the stored counts must not be overwritten.
func TestAddUrgeSurvivesTrackd(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	a, err := s.StartSession(1000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	stale := a // read before the answer: counts 0 in memory
	if err := s.AddUrge(a.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.AddUrge(a.ID, true); err != nil {
		t.Fatal(err)
	}
	stale.DeepMS = 500
	if ok, err := s.UpdateSession(stale); err != nil || !ok {
		t.Fatalf("UpdateSession = %v, %v", ok, err)
	}
	stale.EndMS = 3000
	if ok, err := s.EndSession(stale); err != nil || !ok {
		t.Fatalf("EndSession = %v, %v", ok, err)
	}
	got, err := s.SessionByID(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.UrgesResisted != 1 || got.UrgesGiven != 1 || got.DeepMS != 500 {
		t.Fatalf("session = %+v, want 1 resisted, 1 given, deep 500", got)
	}
}
