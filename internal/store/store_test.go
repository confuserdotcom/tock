package store

import (
	"path/filepath"
	"testing"

	"tock/internal/span"
)

func dbPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "tock.db")
}

func openAt(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func apply(t *testing.T, s *Store, ops ...span.Op) {
	t.Helper()
	if err := s.Apply(ops); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}

func openOp(id, start int64) span.Op {
	return span.Op{Kind: span.OpOpen, Span: span.Span{ID: id, Start: start, BundleID: "com.example.app", App: "Example"}}
}

func hbOp(ts int64) span.Op { return span.Op{Kind: span.OpHeartbeat, TS: ts} }

// spanEnd returns the end of span id and whether it is non-NULL.
func spanEnd(t *testing.T, s *Store, id int64) (int64, bool) {
	t.Helper()
	var end *int64
	if err := s.db.QueryRow(`SELECT "end" FROM span WHERE id=?`, id).Scan(&end); err != nil {
		t.Fatalf("select end of %d: %v", id, err)
	}
	if end == nil {
		return 0, false
	}
	return *end, true
}

func recoverOnce(t *testing.T, s *Store) int64 {
	t.Helper()
	n, err := s.Recover()
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	return n
}

// TestRecoverClosesOpenSpanAtHeartbeat is the crash-recovery tracer: the store
// is abandoned with a span open, reopened, and the span ends at the heartbeat.
func TestRecoverClosesOpenSpanAtHeartbeat(t *testing.T) {
	path := dbPath(t)
	s := openAt(t, path)
	apply(t, s, openOp(1, 1000))
	apply(t, s, hbOp(9000))
	if err := s.Close(); err != nil { // no Close op: simulated crash
		t.Fatal(err)
	}

	s = openAt(t, path)
	defer s.Close()
	if n := recoverOnce(t, s); n != 1 {
		t.Fatalf("Recover count = %d, want 1", n)
	}
	if end, ok := spanEnd(t, s, 1); !ok || end != 9000 {
		t.Fatalf("end = %d (set=%v), want 9000", end, ok)
	}
}

// TestRecoverHeartbeatOlderThanStart: the heartbeat predates the open span, so
// the span is closed at zero length rather than with end < start.
func TestRecoverHeartbeatOlderThanStart(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s, hbOp(500))
	apply(t, s, openOp(1, 1000))
	if n := recoverOnce(t, s); n != 1 {
		t.Fatalf("Recover count = %d, want 1", n)
	}
	if end, ok := spanEnd(t, s, 1); !ok || end != 1000 {
		t.Fatalf("end = %d (set=%v), want 1000 (start)", end, ok)
	}
}

func TestRecoverNoHeartbeat(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s, openOp(1, 1000))
	if n := recoverOnce(t, s); n != 1 {
		t.Fatalf("Recover count = %d, want 1", n)
	}
	if end, ok := spanEnd(t, s, 1); !ok || end != 1000 {
		t.Fatalf("end = %d (set=%v), want 1000 (start)", end, ok)
	}
}

// TestRecoverCleanShutdownNoop: a closed span is never touched.
func TestRecoverCleanShutdownNoop(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s, openOp(1, 1000), hbOp(9000), span.Op{Kind: span.OpClose, ID: 1, TS: 2000})
	if n := recoverOnce(t, s); n != 0 {
		t.Fatalf("Recover count = %d, want 0", n)
	}
	if end, ok := spanEnd(t, s, 1); !ok || end != 2000 {
		t.Fatalf("end = %d (set=%v), want 2000 unchanged", end, ok)
	}
}

// TestHeartbeatUpsert: heartbeats upsert a single daemon_state row.
func TestHeartbeatUpsert(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s, hbOp(1000))
	apply(t, s, hbOp(8500))
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM daemon_state WHERE key=?`, KeyHeartbeat).Scan(&n); err != nil || n != 1 {
		t.Fatalf("heartbeat rows = %d (err %v), want 1", n, err)
	}
	var v string
	if err := s.db.QueryRow(`SELECT value FROM daemon_state WHERE key=?`, KeyHeartbeat).Scan(&v); err != nil || v != "8500" {
		t.Fatalf("heartbeat value = %q (err %v), want 8500", v, err)
	}
}
