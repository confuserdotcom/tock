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
	a.Wakes, a.WakesBack = 2, 1
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

func TestSessionSwitchesPersist(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	a, err := s.StartSession(1000, 1000+25*60_000)
	if err != nil {
		t.Fatal(err)
	}
	a.EndMS, a.Switches = 9000, 4
	if ok, err := s.EndSession(a); err != nil || !ok {
		t.Fatalf("EndSession = %v, %v", ok, err)
	}
	if ok, err := s.AddAutoSession(Session{StartMS: 20_000, EndMS: 30_000, Switches: 2}); err != nil || !ok {
		t.Fatalf("AddAutoSession = %v, %v", ok, err)
	}
	got, err := s.Sessions(0, 100_000)
	if err != nil || len(got) != 2 {
		t.Fatalf("Sessions = %+v, %v", got, err)
	}
	for i, want := range []int{4, 2} {
		x := got[i]
		if x.Switches != want || x.Rating != 0 || x.Note != "" || x.RatedMS != 0 {
			t.Errorf("session %d = %+v, want switches %d and no rating", i, x, want)
		}
	}
}

// A session from before migration 0014 reads back through Sessions() with
// Switches -1.
func TestSessionLegacyRowReadsMinusOne(t *testing.T) {
	path := dbPath(t)
	s := openAt(t, path)
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO session(kind, start_ms, planned_ms, end_ms, switches) VALUES('manual', 1000, 5000, 5000, -1)`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Sessions(0, 10_000)
	if err != nil || len(got) != 1 || got[0].Switches != -1 || got[0].Rating != 0 {
		t.Fatalf("Sessions = %+v, %v", got, err)
	}
}

// finished inserts a closed manual session and returns it.
func finished(t *testing.T, s *Store, start, end int64) Session {
	t.Helper()
	x, err := s.StartSession(start, start+60_000)
	if err != nil {
		t.Fatal(err)
	}
	x.EndMS, x.DeepMS, x.DistractMS, x.LongestMS, x.Interruptions, x.Switches = end, 11, 12, 13, 2, 3
	if ok, err := s.EndSession(x); err != nil || !ok {
		t.Fatalf("EndSession = %v, %v", ok, err)
	}
	return x
}

func TestRateSessionGuards(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	run, err := s.StartSession(1000, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RateSession(run.ID, 4, strp("x"), 5000); !errors.Is(err, ErrSessionOpen) {
		t.Fatalf("rate running = %v, want ErrSessionOpen", err)
	}
	if err := s.SetNote(run.ID, "x"); !errors.Is(err, ErrSessionOpen) {
		t.Fatalf("note running = %v, want ErrSessionOpen", err)
	}
	if got, _ := s.SessionByID(run.ID); got.Rating != 0 || got.Note != "" || got.RatedMS != 0 {
		t.Fatalf("running session changed: %+v", got)
	}
	if err := s.RateSession(999, 4, nil, 5000); !errors.Is(err, ErrSessionGone) {
		t.Fatalf("rate 999 = %v, want ErrSessionGone", err)
	}
	if err := s.SetNote(999, "x"); !errors.Is(err, ErrSessionGone) {
		t.Fatalf("note 999 = %v, want ErrSessionGone", err)
	}
	if _, err := s.SessionByID(999); !errors.Is(err, ErrSessionGone) {
		t.Fatalf("SessionByID 999 = %v", err)
	}
	run.EndMS = 9000
	if ok, err := s.EndSession(run); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := s.RateSession(run.ID, 4, strp("good run"), 12345); err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionByID(run.ID)
	if err != nil || got.Rating != 4 || got.Note != "good run" || got.RatedMS != 12345 {
		t.Fatalf("rated = %+v, %v", got, err)
	}
}

func TestRateSessionLastWins(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	x := finished(t, s, 1000, 9000)
	if err := s.RateSession(x.ID, 3, strp("first"), 100); err != nil {
		t.Fatal(err)
	}
	if err := s.RateSession(x.ID, 5, nil, 200); err != nil {
		t.Fatal(err)
	}
	got, _ := s.SessionByID(x.ID)
	if got.Rating != 5 || got.RatedMS != 200 || got.Note != "first" {
		t.Fatalf("after nil-note rerate = %+v", got)
	}
	if err := s.RateSession(x.ID, 2, strp("second"), 300); err != nil {
		t.Fatal(err)
	}
	got, _ = s.SessionByID(x.ID)
	if got.Rating != 2 || got.Note != "second" || got.RatedMS != 300 {
		t.Fatalf("after note rerate = %+v", got)
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM session`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows = %d, %v", n, err)
	}
}

func TestRateSessionPersistsReopen(t *testing.T) {
	path := dbPath(t)
	s := openAt(t, path)
	x := finished(t, s, 1000, 9000)
	if err := s.RateSession(x.ID, 4, strp("kept"), 777); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = openAt(t, path)
	defer s.Close()
	got, err := s.SessionByID(x.ID)
	if err != nil || got.Rating != 4 || got.Note != "kept" || got.RatedMS != 777 {
		t.Fatalf("reopened = %+v, %v", got, err)
	}
}

func TestRateSessionCountersUntouched(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	x := finished(t, s, 1000, 9000)
	if err := s.RateSession(x.ID, 4, strp("n"), 5); err != nil {
		t.Fatal(err)
	}
	got, _ := s.SessionByID(x.ID)
	if got.DeepMS != 11 || got.DistractMS != 12 || got.LongestMS != 13 || got.Interruptions != 2 || got.Switches != 3 {
		t.Fatalf("rating altered counters: %+v", got)
	}
	// trackd's writers must not touch a finished row, nor the rating columns.
	x.DeepMS = 999
	if ok, err := s.UpdateSession(x); err != nil || ok {
		t.Fatalf("UpdateSession closed = %v, %v", ok, err)
	}
	if ok, err := s.EndSession(x); err != nil || ok {
		t.Fatalf("EndSession closed = %v, %v", ok, err)
	}
	after, _ := s.SessionByID(x.ID)
	if after != got {
		t.Fatalf("row changed: %+v -> %+v", got, after)
	}
	// and while running, the counter writers leave the rating columns alone.
	run, _ := s.StartSession(20_000, 30_000)
	run.DeepMS = 5
	if ok, err := s.UpdateSession(run); err != nil || !ok {
		t.Fatal(ok, err)
	}
	run.EndMS = 40_000
	if ok, err := s.EndSession(run); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if r, _ := s.SessionByID(run.ID); r.Rating != 0 || r.Note != "" || r.RatedMS != 0 {
		t.Fatalf("counters wrote rating columns: %+v", r)
	}
}

func TestRateSetNote(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	x := finished(t, s, 1000, 9000)
	if err := s.SetNote(x.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.SessionByID(x.ID); got.Note != "hello" {
		t.Fatalf("note = %q", got.Note)
	}
	if err := s.SetNote(x.ID, ""); err != nil {
		t.Fatal(err)
	}
	var isNull bool
	if err := s.db.QueryRow(`SELECT note IS NULL FROM session WHERE id=?`, x.ID).Scan(&isNull); err != nil || !isNull {
		t.Fatalf("note not NULL: %v %v", isNull, err)
	}
	if got, _ := s.SessionByID(x.ID); got.Note != "" {
		t.Fatalf("note = %q", got.Note)
	}
}

func TestRatedSessionsByID(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	a := finished(t, s, 10_000, 20_000)
	b := finished(t, s, 30_000, 40_000)
	c := finished(t, s, 50_000, 60_000)
	if _, err := s.StartSession(70_000, 80_000); err != nil {
		t.Fatal(err)
	}
	for _, x := range []Session{c, a} { // rate out of id order
		if err := s.RateSession(x.ID, 3, nil, 1); err != nil {
			t.Fatal(err)
		}
	}
	_ = b
	got, err := s.RatedSessions()
	if err != nil || len(got) != 2 || got[0].ID != a.ID || got[1].ID != c.ID {
		t.Fatalf("RatedSessions = %+v, %v", got, err)
	}
}

func TestLastFinishedSession(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	if x, err := s.LastFinishedSession(); err != nil || x != nil {
		t.Fatalf("empty = %v, %v", x, err)
	}
	if _, err := s.StartSession(1000, 2000); err != nil {
		t.Fatal(err)
	}
	if x, err := s.LastFinishedSession(); err != nil || x != nil {
		t.Fatalf("running only = %v, %v", x, err)
	}
	// finish it, then add an older-ending one: newest by end_ms wins
	open, _ := s.OpenSession()
	open.EndMS = 9000
	s.EndSession(*open)
	if _, err := s.AddAutoSession(Session{StartMS: 100, EndMS: 5000}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartSession(20_000, 30_000); err != nil {
		t.Fatal(err)
	}
	x, err := s.LastFinishedSession()
	if err != nil || x == nil || x.ID != open.ID {
		t.Fatalf("last = %+v, %v", x, err)
	}
}

func TestStartSessionClearsDistractSnooze(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	if err := s.SetState(KeyFocusDistractSnooze, "999999", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartSession(1000, 2000); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := s.State(KeyFocusDistractSnooze); err != nil || ok {
		t.Fatalf("snooze after StartSession = %q, %v, %v; want gone", v, ok, err)
	}

	// A start that fails leaves a snooze of the running session alone.
	if err := s.SetState(KeyFocusDistractSnooze, "888888", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartSession(3000, 4000); !errors.Is(err, ErrSessionRunning) {
		t.Fatalf("second start err = %v, want ErrSessionRunning", err)
	}
	if v, ok, err := s.State(KeyFocusDistractSnooze); err != nil || !ok || v != "888888" {
		t.Fatalf("snooze after failed start = %q, %v, %v; want kept", v, ok, err)
	}
}

func TestSessionGoal(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	x, err := s.StartSession(1000, 61000)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSessionGoal(x.ID, "TD 3, exercises 4 to 7"); err != nil {
		t.Fatal(err)
	}
	o, err := s.OpenSession()
	if err != nil || o == nil || o.Goal != "TD 3, exercises 4 to 7" {
		t.Fatalf("open session = %+v, %v", o, err)
	}
	if err := s.SetSessionGoal(x.ID+99, "x"); !errors.Is(err, ErrSessionGone) {
		t.Fatalf("unknown session: %v", err)
	}
}
