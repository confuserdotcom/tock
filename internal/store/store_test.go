package store

import (
	"errors"
	"path/filepath"
	"strings"
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

func strp(s string) *string { return &s }

func rowCount(t *testing.T, s *Store, id int64) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM span WHERE id=?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDaemonStateRoundTrip(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	if v, ok, err := s.State(KeyVersion); err != nil || ok || v != "" {
		t.Fatalf("missing key = (%q,%v,%v), want (\"\",false,nil)", v, ok, err)
	}
	if err := s.SetState(KeyVersion, "v1", 1234); err != nil {
		t.Fatal(err)
	}
	v, ok, err := s.State(KeyVersion)
	if err != nil || !ok || v != "v1" {
		t.Fatalf("State = (%q,%v,%v), want (v1,true,nil)", v, ok, err)
	}
	var updated int64
	if err := s.db.QueryRow(`SELECT updated FROM daemon_state WHERE key=?`, KeyVersion).Scan(&updated); err != nil || updated != 1234 {
		t.Fatalf("updated = %d (err %v), want 1234", updated, err)
	}
	// Upsert overwrites value and updated.
	if err := s.SetState(KeyVersion, "v2", 5678); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := s.State(KeyVersion); v != "v2" {
		t.Fatalf("after upsert value = %q, want v2", v)
	}
	if err := s.db.QueryRow(`SELECT updated FROM daemon_state WHERE key=?`, KeyVersion).Scan(&updated); err != nil || updated != 5678 {
		t.Fatalf("updated after upsert = %d (err %v), want 5678", updated, err)
	}
}

func TestDaemonStateDegradeKeys(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	for _, kv := range [][2]string{
		{KeyAXTrusted, "0"},
		{AutomationKey("com.google.Chrome"), "denied"},
		{AutomationKey("com.apple.Safari"), "granted"},
	} {
		if err := s.SetState(kv[0], kv[1], 100); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.States()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"ax_trusted":                   "0",
		"automation.com.google.Chrome": "denied",
		"automation.com.apple.Safari":  "granted",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("States()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestSetStateIsParameterized(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	evil := `x'); DROP TABLE span; --`
	if err := s.SetState(evil, evil, 1); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := s.State(evil); !ok || v != evil {
		t.Fatalf("hostile key not stored verbatim: (%q,%v)", v, ok)
	}
	if _, err := s.NextID(); err != nil {
		t.Fatalf("span table damaged: %v", err)
	}
}

func TestApplyEveryOpKind(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s,
		openOp(1, 1000),
		openOp(2, 3000),
		openOp(3, 5000),
		span.Op{Kind: span.OpClose, ID: 1, TS: 2000},
		span.Op{Kind: span.OpClose, ID: 2, TS: 4000},
		span.Op{Kind: span.OpExtend, ID: 1, TS: 2500},
		span.Op{Kind: span.OpReopen, ID: 2},
		span.Op{Kind: span.OpDelete, ID: 3},
		hbOp(7000),
	)
	if end, ok := spanEnd(t, s, 1); !ok || end != 2500 {
		t.Errorf("span 1 end = %d (set=%v), want 2500 (extended)", end, ok)
	}
	if _, ok := spanEnd(t, s, 2); ok {
		t.Errorf("span 2 end set, want NULL (reopened)")
	}
	if rowCount(t, s, 3) != 0 {
		t.Errorf("span 3 still present, want deleted")
	}
	if v, ok, _ := s.State(KeyHeartbeat); !ok || v != "7000" {
		t.Errorf("heartbeat = (%q,%v), want 7000", v, ok)
	}
}

func TestApplyAtomicRollback(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()

	// Duplicate primary key on the second op.
	if err := s.Apply([]span.Op{openOp(5, 100), openOp(5, 200)}); err == nil {
		t.Fatal("duplicate id: Apply returned nil, want error")
	}
	if n := rowCount(t, s, 5); n != 0 {
		t.Fatalf("id 5 rows = %d after failed Apply, want 0", n)
	}

	// Unknown op kind on the second op.
	err := s.Apply([]span.Op{openOp(6, 100), {Kind: span.OpKind(99)}})
	if !errors.Is(err, ErrUnknownOp) {
		t.Fatalf("unknown kind err = %v, want ErrUnknownOp", err)
	}
	if n := rowCount(t, s, 6); n != 0 {
		t.Fatalf("id 6 rows = %d after failed Apply, want 0", n)
	}

	// The store is still usable after a rollback.
	apply(t, s, openOp(7, 100))
	if rowCount(t, s, 7) != 1 {
		t.Fatal("store unusable after rollback")
	}
}

func TestApplyErrorNamesOpIndexAndKind(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	err := s.Apply([]span.Op{openOp(1, 100), openOp(1, 200)})
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "op 1") || !strings.Contains(msg, "open") {
		t.Fatalf("error %q should name op index 1 and kind open", msg)
	}
}

func TestApplyPrivateForcesNullTitleURL(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s, span.Op{Kind: span.OpOpen, Span: span.Span{
		ID: 1, Start: 100, BundleID: "com.example.app", App: "Example",
		Title: strp("secret"), URL: strp("https://secret.example"), Private: true,
	}})
	var title, url *string
	var priv int
	if err := s.db.QueryRow(`SELECT title,url,private FROM span WHERE id=1`).Scan(&title, &url, &priv); err != nil {
		t.Fatal(err)
	}
	if title != nil || url != nil || priv != 1 {
		t.Fatalf("private span stored title=%v url=%v private=%d, want NULL NULL 1", title, url, priv)
	}
	// A non-private span keeps its values.
	apply(t, s, span.Op{Kind: span.OpOpen, Span: span.Span{
		ID: 2, Start: 200, BundleID: "com.example.app", App: "Example",
		Title: strp("t"), URL: strp("u"),
	}})
	if err := s.db.QueryRow(`SELECT title,url FROM span WHERE id=2`).Scan(&title, &url); err != nil {
		t.Fatal(err)
	}
	if title == nil || *title != "t" || url == nil || *url != "u" {
		t.Fatalf("non-private span lost title/url: %v %v", title, url)
	}
}

func TestApplyDefaultsSourceNoneAndNullClassification(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	apply(t, s, openOp(1, 100))
	var source string
	var cat, proj, rule *int64
	if err := s.db.QueryRow(`SELECT source,category_id,project_id,rule_id FROM span WHERE id=1`).Scan(&source, &cat, &proj, &rule); err != nil {
		t.Fatal(err)
	}
	if source != "none" || cat != nil || proj != nil || rule != nil {
		t.Fatalf("source=%q cat=%v proj=%v rule=%v, want none and NULLs", source, cat, proj, rule)
	}
}

func TestNextIDAfterDelete(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	if n, err := s.NextID(); err != nil || n != 1 {
		t.Fatalf("empty NextID = %d (err %v), want 1", n, err)
	}
	apply(t, s, openOp(1, 100), openOp(2, 200), openOp(3, 300))
	apply(t, s, span.Op{Kind: span.OpDelete, ID: 3}, span.Op{Kind: span.OpDelete, ID: 1})
	if n, err := s.NextID(); err != nil || n != 3 {
		t.Fatalf("NextID = %d (err %v), want MAX(id)+1 = 3", n, err)
	}
}
