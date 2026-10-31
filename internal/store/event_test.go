package store

import (
	"path/filepath"
	"testing"
)

func TestSyncEvents(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	old := Event{StartMS: 100, EndMS: 200, Title: "old", Calendar: "Work"}
	if ch, err := s.SyncEvents(0, 1000, []Event{old}); err != nil || !ch {
		t.Fatalf("first sync: %v %v", ch, err)
	}
	a := Event{StartMS: 1100, EndMS: 1200, Title: "standup", Calendar: "Work"}
	b := Event{StartMS: 1500, EndMS: 1900, Title: "review", Calendar: "Home"}
	out := Event{StartMS: 2500, EndMS: 2600, Title: "outside"}
	if ch, err := s.SyncEvents(1000, 2000, []Event{b, a, out}); err != nil || !ch {
		t.Fatalf("sync: %v %v", ch, err)
	}
	n0, id0, _ := s.EventStamp()
	// The same set in another order writes nothing.
	if ch, err := s.SyncEvents(1000, 2000, []Event{a, b}); err != nil || ch {
		t.Fatalf("equal sync: changed %v, %v", ch, err)
	}
	if n, id, _ := s.EventStamp(); n != n0 || id != id0 || n != 3 {
		t.Fatalf("stamp moved on an equal sync: %d/%d vs %d/%d", n, id, n0, id0)
	}
	// b moved: only the window is replaced, the old row stays.
	b.EndMS = 1800
	if ch, err := s.SyncEvents(1000, 2000, []Event{a, b}); err != nil || !ch {
		t.Fatalf("changed sync: %v %v", ch, err)
	}
	if _, id, _ := s.EventStamp(); id == id0 {
		t.Fatal("stamp did not move")
	}
	evs, err := s.Events(0, 3000)
	if err != nil || len(evs) != 3 || evs[0].Title != "old" || evs[2].EndMS != 1800 || evs[1].Calendar != "Work" {
		t.Fatalf("events = %+v, %v", evs, err)
	}
	if evs, _ := s.Events(1150, 1160); len(evs) != 1 || evs[0].Title != "standup" {
		t.Fatalf("overlap query = %+v", evs)
	}
	// An empty window clears it.
	if ch, err := s.SyncEvents(1000, 2000, nil); err != nil || !ch {
		t.Fatalf("clear: %v %v", ch, err)
	}
	if evs, _ := s.Events(0, 3000); len(evs) != 1 {
		t.Fatalf("after clear = %+v", evs)
	}
}

func TestSyncEventsJoinURL(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const link = "https://meet.google.com/abc-defg-hij"
	e := Event{StartMS: 1100, EndMS: 1200, Title: "standup", Calendar: "Work", JoinURL: link}
	if ch, err := s.SyncEvents(1000, 2000, []Event{e}); err != nil || !ch {
		t.Fatalf("sync: %v %v", ch, err)
	}
	evs, err := s.Events(0, 3000)
	if err != nil || len(evs) != 1 || evs[0].JoinURL != link {
		t.Fatalf("Events = %+v, %v, want the link", evs, err)
	}
	one, err := s.Event(evs[0].ID)
	if err != nil || one.JoinURL != link {
		t.Fatalf("Event = %+v, %v, want the link", one, err)
	}
	if ch, err := s.SyncEvents(1000, 2000, []Event{e}); err != nil || ch {
		t.Fatalf("same events: changed %v, %v, want false", ch, err)
	}
	e.JoinURL = "https://zoom.us/j/1"
	if ch, err := s.SyncEvents(1000, 2000, []Event{e}); err != nil || !ch {
		t.Fatalf("other link: changed %v, %v, want true", ch, err)
	}
	if evs, _ := s.Events(0, 3000); len(evs) != 1 || evs[0].JoinURL != "https://zoom.us/j/1" {
		t.Fatalf("Events after relink = %+v", evs)
	}
	if _, err := s.db.Exec(`INSERT INTO event(start_ms, end_ms, title, calendar) VALUES(1500, 1600, 'raw', 'Work')`); err != nil {
		t.Fatal(err)
	}
	evs, _ = s.Events(1400, 1700)
	if len(evs) != 1 || evs[0].JoinURL != "" {
		t.Fatalf("raw row = %+v, want an empty link", evs)
	}
}
