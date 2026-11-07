package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tock/internal/collect"
	"tock/internal/store"
)

// The worker opens (creating or migrating) the database and exits 0.
func TestMigrateWorkerMigrates(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if rc := runMigrateWorker([]string{"--data", dir}, &out); rc != 0 {
		t.Fatalf("rc = %d: %s", rc, out.String())
	}
	st, err := store.Open(filepath.Join(dir, "tock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	v, err := st.SchemaVersion()
	if err != nil || v != store.LatestVersion() {
		t.Fatalf("version = %d (%v), want %d", v, err, store.LatestVersion())
	}
}

// A database newer than the binary is refused and left as it is; bad
// arguments are exit 2.
func TestMigrateWorkerRefuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tock.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version(version, applied_at) VALUES(999, 0)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var out bytes.Buffer
	if rc := runMigrateWorker([]string{"--data", dir}, &out); rc != 1 {
		t.Fatalf("too new: rc = %d, want 1: %s", rc, out.String())
	}
	if _, err := store.Open(path); !errors.Is(err, store.ErrSchemaTooNew) {
		t.Fatalf("after the worker: %v, want still too new", err)
	}
	if rc := runMigrateWorker(nil, &out); rc != 2 {
		t.Fatalf("no args: rc = %d, want 2", rc)
	}
}

// trackd runs the child before it opens the store itself, and a child
// that fails changes nothing: trackd migrates in-process as before.
func TestDaemonMigratesInChildFirst(t *testing.T) {
	for _, fail := range []bool{false, true} {
		dir := t.TempDir()
		var calls []string
		var buf bytes.Buffer
		rc := runDaemon(daemonOpts{
			DataDir: dir, StateDir: dir, Stderr: &buf,
			NewSource: func() (collect.Source, error) { return samplesSource(), nil },
			Premigrate: func(data string) error {
				_, err := os.Stat(filepath.Join(data, "tock.db"))
				calls = append(calls, fmt.Sprintf("%s db=%v", data, err == nil))
				if fail {
					return errors.New("child failed")
				}
				return nil
			},
		})
		if rc != 0 {
			t.Fatalf("fail=%v: rc = %d\n%s", fail, rc, buf.String())
		}
		if len(calls) != 1 || calls[0] != dir+" db=false" {
			t.Fatalf("fail=%v: calls = %q, want one before the store was opened", fail, calls)
		}
		if fail != strings.Contains(buf.String(), "migrating in-process") {
			t.Fatalf("fail=%v: log:\n%s", fail, buf.String())
		}
		st, err := store.Open(filepath.Join(dir, "tock.db"))
		if err != nil {
			t.Fatal(err)
		}
		if v, _ := st.SchemaVersion(); v != store.LatestVersion() {
			t.Fatalf("fail=%v: version %d", fail, v)
		}
		st.Close()
	}
}
