package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/store"
)

func rawExec(t *testing.T, path, q string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func rawState(t *testing.T, path, key string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v string
	_ = db.QueryRow(`SELECT value FROM daemon_state WHERE key = ?`, key).Scan(&v)
	return v
}

// A newer tock's additive migration leaves compat alone: trackd starts.
func TestTrackdRunsOnNewerCompatibleSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	rawExec(t, path, `INSERT INTO schema_version(version, applied_at) VALUES(99, 0)`)
	r := runFakeDaemon(t, dir, samplesSource())
	if r.rc != 0 || strings.Contains(r.log, "FATAL") {
		t.Fatalf("rc %d log:\n%s", r.rc, r.log)
	}
}

// A database trackd refuses at start says so to tock status.
func TestTrackdMarksStoppedOnHigherCompat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	rawExec(t, path, `UPDATE schema_meta SET value = ? WHERE key = 'compat'`, store.CompatLevel+1)
	r := runFakeDaemon(t, dir, samplesSource())
	if r.rc != exitNoRestart {
		t.Fatalf("rc = %d", r.rc)
	}
	if got := rawState(t, path, store.KeyTrackdStopped); got != store.StoppedSchemaNewer {
		b, _ := os.ReadFile(filepath.Join(dir, "trackd.log"))
		t.Fatalf("trackd.stopped = %q; log:\n%s%s", got, r.log, b)
	}
}

// A good start clears a stop reason an earlier trackd left.
func TestTrackdStartClearsStopped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.SetState(store.KeyTrackdStopped, store.StoppedSchemaNewer, 1)
	st.Close()
	runFakeDaemon(t, dir, samplesSource())
	if got := rawState(t, path, store.KeyTrackdStopped); got != "" {
		t.Fatalf("trackd.stopped = %q after a good start", got)
	}
}

// A tock that raises compat under a running trackd stops it at the next
// tick, with the reason on its run row.
func TestTrackdStopsWhenCompatRaisedWhileRunning(t *testing.T) {
	r := newPauseRig(t, "")
	r.p.rec = &runRec{st: r.st}
	if err := r.p.start(); err != nil {
		t.Fatal(err)
	}
	if err := r.p.tickAt(pauseT0 + 10_000); err != nil {
		t.Fatalf("tick before the raise: %v", err)
	}
	rawExec(t, r.path, `UPDATE schema_meta SET value = ? WHERE key = 'compat'`, store.CompatLevel+1)
	err := r.p.tickAt(pauseT0 + 20_000)
	if !errors.Is(err, errSchemaNewer) {
		t.Fatalf("tick after the raise: %v, want errSchemaNewer", err)
	}
}
