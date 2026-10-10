package store

import (
	"errors"
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()

	if o, err := s.OpenSession(); err != nil || o != nil {
		t.Fatalf("OpenSession on empty = %v, %v", o, err)
	}
	a, err := s.StartSession(1000, 1000+25*60_000)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || a.Kind != SessionManual || a.EndMS != 0 {
		t.Fatalf("started = %+v", a)
	}
	if _, err := s.StartSession(2000, 3000); !errors.Is(err, ErrSessionRunning) {
		t.Fatalf("second start err = %v, want ErrSessionRunning", err)
	}

	a.Interruptions, a.AlertedMS, a.DeepMS, a.DistractMS, a.LongestMS = 1, 70_000, 5, 6, 7
	if ok, err := s.UpdateSession(a); err != nil || !ok {
		t.Fatalf("UpdateSession = %v, %v", ok, err)
	}
	o, err := s.OpenSession()
	if err != nil || o == nil || *o != a {
		t.Fatalf("OpenSession = %+v, %v; want %+v", o, err, a)
	}

	a.EndMS = 9000
	if ok, err := s.EndSession(a); err != nil || !ok {
		t.Fatalf("EndSession = %v, %v", ok, err)
	}
	if ok, err := s.EndSession(a); err != nil || ok {
		t.Fatalf("second EndSession = %v, %v; want false", ok, err)
	}
	if ok, err := s.UpdateSession(a); err != nil || ok {
		t.Fatalf("UpdateSession of a closed session = %v, %v; want false", ok, err)
	}
	if o, err := s.OpenSession(); err != nil || o != nil {
		t.Fatalf("OpenSession after end = %v, %v", o, err)
	}
	if _, err := s.StartSession(10_000, 20_000); err != nil {
		t.Fatalf("start after end: %v", err)
	}
}

func TestAutoSessionsAndList(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()

	auto := Session{Kind: SessionAuto, StartMS: 100, EndMS: 200, DeepMS: 90, DistractMS: 5, LongestMS: 80}
	if ok, err := s.AddAutoSession(auto); err != nil || !ok {
		t.Fatalf("AddAutoSession = %v, %v", ok, err)
	}
	if ok, err := s.AddAutoSession(auto); err != nil || ok {
		t.Fatalf("duplicate AddAutoSession = %v, %v; want false", ok, err)
	}
	m, err := s.StartSession(300, 400)
	if err != nil {
		t.Fatal(err)
	}

	all, err := s.Sessions(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	auto.ID = all[0].ID
	if len(all) != 2 || all[0] != auto || all[1] != m {
		t.Fatalf("Sessions = %+v", all)
	}
	// The running session overlaps any later window; the auto one ended at 200.
	later, err := s.Sessions(250, 5000)
	if err != nil || len(later) != 1 || later[0].ID != m.ID {
		t.Fatalf("Sessions(250, 5000) = %+v, %v", later, err)
	}
	if none, err := s.Sessions(0, 100); err != nil || len(none) != 0 {
		t.Fatalf("Sessions(0, 100) = %+v, %v", none, err)
	}
}
