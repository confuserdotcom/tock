package store

import (
	"testing"

	"tock/internal/span"
)

func TestOpenBundle(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	if _, open, err := s.OpenBundle(); err != nil || open {
		t.Fatalf("empty: open=%v err=%v", open, err)
	}
	if _, err := s.db.Exec(`INSERT INTO span(id, start, "end", bundle_id, app) VALUES(1, 1000, NULL, ?, 'idle')`, span.IdleBundleID); err != nil {
		t.Fatal(err)
	}
	b, open, err := s.OpenBundle()
	if err != nil || !open || b != span.IdleBundleID {
		t.Fatalf("got %q %v %v", b, open, err)
	}
}
