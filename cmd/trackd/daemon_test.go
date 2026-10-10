package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/collect"
	"tock/internal/span"
	"tock/internal/store"
)

type daemonRun struct {
	rc  int
	log string
	dir string
}

// runFakeDaemon runs runDaemon on dir (data and state) with src.
func runFakeDaemon(t *testing.T, dir string, src collect.Source) daemonRun {
	t.Helper()
	var buf bytes.Buffer
	rc := runDaemon(daemonOpts{
		DataDir:   dir,
		StateDir:  dir,
		NewSource: func() (collect.Source, error) { return src, nil },
		Stderr:    &buf,
	})
	return daemonRun{rc: rc, log: buf.String(), dir: dir}
}

func samplesSource(ss ...span.Sample) collect.Source {
	ins := make([]span.Input, len(ss))
	for i := range ss {
		ins[i] = span.Input{Sample: &ss[i]}
	}
	return collect.NewFake(ins)
}

func readState(t *testing.T, dir string) map[string]string {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m, err := st.States()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestLockHeldExitsZero(t *testing.T) {
	dir := t.TempDir()
	release, err := acquireLock(filepath.Join(dir, "trackd.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	r := runFakeDaemon(t, dir, samplesSource())
	if r.rc != 0 {
		t.Fatalf("rc = %d; want 0", r.rc)
	}
	if !strings.Contains(r.log, "FATAL") || !strings.Contains(r.log, "already running") {
		t.Fatalf("log missing FATAL/already running:\n%s", r.log)
	}
	if _, err := os.Stat(filepath.Join(dir, "tock.db")); !os.IsNotExist(err) {
		t.Fatalf("database opened while the lock was held (stat err %v)", err)
	}
}

func TestSchemaTooNewExitsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version(version, applied_at) VALUES(99, 0)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	r := runFakeDaemon(t, dir, samplesSource())
	if r.rc != 0 {
		t.Fatalf("rc = %d; want 0", r.rc)
	}
	if !strings.Contains(r.log, "FATAL") || !strings.Contains(r.log, "not restarting") {
		t.Fatalf("log missing FATAL/not restarting:\n%s", r.log)
	}
}

// staleSource is a Source whose Run reports a stuck Accessibility grant.
type staleSource struct{ ch chan span.Input }

func (s *staleSource) Inputs() <-chan span.Input { return s.ch }
func (s *staleSource) Run() error                { close(s.ch); return collect.ErrAXStale }
func (s *staleSource) Stop()                     {}
func (s *staleSource) Dropped() uint64           { return 0 }

func TestAXStaleExitsOne(t *testing.T) {
	r := runFakeDaemon(t, t.TempDir(), &staleSource{ch: make(chan span.Input)})
	if r.rc != 1 {
		t.Fatalf("rc = %d; want 1\n%s", r.rc, r.log)
	}
	if !strings.Contains(r.log, "accessibility grant stale") {
		t.Fatalf("log missing stale line:\n%s", r.log)
	}
}

func TestStateWritesOnTransitions(t *testing.T) {
	const t0 = int64(1700000000000)
	const arc = "company.thebrowser.Browser"
	mk := func(i int64, ax bool, auto string) span.Sample {
		return span.Sample{TS: t0 + i*2000, BundleID: arc, App: "Arc", AXTrusted: ax, Automation: auto}
	}
	r := runFakeDaemon(t, t.TempDir(), samplesSource(
		mk(0, true, span.AutomationGranted),
		mk(1, false, span.AutomationGranted),
		mk(2, false, span.AutomationDenied),
		mk(3, true, span.AutomationDenied),
	))
	if r.rc != 0 {
		t.Fatalf("rc = %d\n%s", r.rc, r.log)
	}
	st := readState(t, r.dir)
	if st[store.KeyAXTrusted] != "1" {
		t.Errorf("ax_trusted = %q; want 1", st[store.KeyAXTrusted])
	}
	if got := st[store.AutomationKey(arc)]; got != span.AutomationDenied {
		t.Errorf("automation.%s = %q; want denied", arc, got)
	}
	if n := strings.Count(r.log, `msg="accessibility `); n != 3 {
		t.Errorf("accessibility log lines = %d; want 3\n%s", n, r.log)
	}
	if n := strings.Count(r.log, `msg="automation status changed"`); n != 2 {
		t.Errorf("automation log lines = %d; want 2\n%s", n, r.log)
	}
	if !strings.Contains(r.log, "Privacy & Security > Accessibility") {
		t.Errorf("missing-grant line does not name the settings path\n%s", r.log)
	}
}

func TestLogNeverContainsTitlesOrURLs(t *testing.T) {
	const t0 = int64(1700000000000)
	title, url := "SECRET-TITLE-123", "secret.example/path"
	r := runFakeDaemon(t, t.TempDir(), samplesSource(
		span.Sample{TS: t0, BundleID: "com.google.Chrome", App: "Chrome", Title: &title, URL: &url,
			AXTrusted: true, Automation: span.AutomationGranted},
		span.Sample{TS: t0 + 3000, BundleID: "com.google.Chrome", App: "Chrome", Title: &title, URL: &url,
			AXTrusted: false, Automation: span.AutomationDenied},
	))
	if r.rc != 0 {
		t.Fatalf("rc = %d\n%s", r.rc, r.log)
	}
	if strings.Contains(r.log, title) || strings.Contains(r.log, url) {
		t.Fatalf("log leaks title or URL:\n%s", r.log)
	}
}

func TestTruncateLogIfLarge(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.log")
	small := filepath.Join(dir, "small.log")
	if err := os.WriteFile(big, make([]byte, 11<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(small, make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{big, small, filepath.Join(dir, "missing.log")} {
		if err := truncateLogIfLarge(p, logMaxBytes); err != nil {
			t.Fatalf("truncateLogIfLarge(%s): %v", p, err)
		}
	}
	if fi, _ := os.Stat(big); fi.Size() != 0 {
		t.Errorf("11 MiB log size = %d; want 0", fi.Size())
	}
	if fi, _ := os.Stat(small); fi.Size() != 1024 {
		t.Errorf("1 KiB log size = %d; want 1024", fi.Size())
	}
}

func TestVersionWrittenAtStartup(t *testing.T) {
	r := runFakeDaemon(t, t.TempDir(), samplesSource())
	if r.rc != 0 {
		t.Fatalf("rc = %d\n%s", r.rc, r.log)
	}
	if got := readState(t, r.dir)[store.KeyVersion]; got != version {
		t.Fatalf("version = %q; want %q", got, version)
	}
}

func TestDropReporterRateLimited(t *testing.T) {
	var buf bytes.Buffer
	d := &dropReporter{log: newLogger(&buf)}
	d.report(0, 0)
	d.report(5, 1000)
	d.report(9, 30000)  // within 60 s of the first warning
	d.report(9, 61000)  // allowed again
	d.report(9, 200000) // no new drops
	if n := strings.Count(buf.String(), "samples dropped"); n != 2 {
		t.Fatalf("drop warnings = %d; want 2\n%s", n, buf.String())
	}
}
