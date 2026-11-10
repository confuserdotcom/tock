package main

import (
	"io"
	"path/filepath"
	"strconv"
	"testing"

	"tock/internal/span"
	"tock/internal/store"
)

// AC2 (trackd half): a write that fails stops the run as write_failed,
// and the next start counts it with the error and when.
func TestWriteFailureCountedAtNextStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	rawExec(t, path, `CREATE TRIGGER fail_span BEFORE INSERT ON span BEGIN SELECT RAISE(ABORT, 'disk on fire'); END`)

	r := runFakeDaemon(t, dir, samplesSource(span.Sample{TS: 1_700_000_000_000, BundleID: "a", App: "A"}))
	if r.rc != exitRestart {
		t.Fatalf("rc = %d, want restart\n%s", r.rc, r.log)
	}
	rawExec(t, path, `DROP TRIGGER fail_span`)
	runFakeDaemon(t, dir, samplesSource())

	s := readState(t, dir)
	if s[store.KeyApplyFailed] != "1" {
		t.Fatalf("%s = %q, want 1", store.KeyApplyFailed, s[store.KeyApplyFailed])
	}
	if ms, _ := strconv.ParseInt(s[store.KeyLastErrorMS], 10, 64); ms <= 0 {
		t.Fatalf("%s = %q", store.KeyLastErrorMS, s[store.KeyLastErrorMS])
	}
	if s[store.KeyLastError] == "" {
		t.Fatal("last_error empty")
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runs, err := st.Runs(0, 1<<62)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range runs {
		found = found || r.Reason == store.RunWriteFailed
	}
	if !found {
		t.Fatalf("runs = %+v, want one stopped as %s", runs, store.RunWriteFailed)
	}
}

func TestDropsCounted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	d := &dropReporter{log: newLogger(io.Discard), st: st}
	d.report(0, 1000)
	d.report(4, 2000)
	d.report(4, 3000)
	d.report(7, 4000)
	v, _, _ := st.State(store.KeySamplesDropped)
	at, _, _ := st.State(store.KeyDroppedMS)
	if v != "7" || at != "4000" {
		t.Fatalf("dropped %q at %q, want 7 at 4000", v, at)
	}
	// A new process starts its source's count at 0 again; the total goes on.
	d = &dropReporter{log: newLogger(io.Discard), st: st}
	d.report(2, 5000)
	if v, _, _ := st.State(store.KeySamplesDropped); v != "9" {
		t.Fatalf("after restart dropped = %q, want 9", v)
	}
}
