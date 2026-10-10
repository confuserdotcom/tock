package store

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
)

func versionRows(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_version ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

func objectExists(t *testing.T, db *sql.DB, typ, name string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type=? AND name=?`, typ, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestMigrateFreshCreatesLatest(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	if got := versionRows(t, s.db); !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Fatalf("schema_version rows = %v, want [1 2 3 4]", got)
	}
	for _, tbl := range []string{"span", "daemon_state", "override", "inbox_ignore", "session", "suggestion"} {
		if !objectExists(t, s.db, "table", tbl) {
			t.Errorf("table %s missing", tbl)
		}
	}
	for _, idx := range []string{"span_start", "span_open"} {
		if !objectExists(t, s.db, "index", idx) {
			t.Errorf("index %s missing", idx)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	path := dbPath(t)
	s := openAt(t, path)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openAt(t, path)
	defer s.Close()
	if got := versionRows(t, s.db); len(got) != 4 {
		t.Fatalf("schema_version rows after reopen = %v, want exactly one per migration", got)
	}
}

// TestOpenTooNewSchema: a database written by a newer trackd is refused and
// left untouched (D-23).
func TestOpenTooNewSchema(t *testing.T) {
	path := dbPath(t)
	s := openAt(t, path)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO schema_version(version, applied_at) VALUES(99, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if !errors.Is(err, ErrSchemaTooNew) {
		if s != nil {
			s.Close()
		}
		t.Fatalf("Open err = %v, want ErrSchemaTooNew", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("tock.db bytes changed while refusing a newer schema")
	}
	raw, err = sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if got := versionRows(t, raw); !slices.Equal(got, []int{1, 2, 3, 4, 99}) {
		t.Fatalf("schema_version rows = %v, want [1 2 3 4 99] untouched", got)
	}
}

func TestSchemaVersion(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	if v, err := s.SchemaVersion(); err != nil || v != 4 {
		t.Fatalf("SchemaVersion = %d (err %v), want 4", v, err)
	}
}

func TestPermsDirAndFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub")
	path := filepath.Join(dir, "tock.db")
	s := openAt(t, path)
	defer s.Close()
	apply(t, s, openOp(1, 100))

	mode := func(p string) os.FileMode {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		return fi.Mode().Perm()
	}
	if m := mode(dir); m != 0o700 {
		t.Errorf("data dir mode = %o, want 700", m)
	}
	if m := mode(path); m != 0o600 {
		t.Errorf("tock.db mode = %o, want 600", m)
	}
	// The WAL must exist after a write; -shm exists while a connection is open.
	for _, suffix := range []string{"-wal", "-shm"} {
		if m := mode(path + suffix); m != 0o600 {
			t.Errorf("tock.db%s mode = %o, want 600", suffix, m)
		}
	}
}

// TestPermsTightensLooseExistingDir: a pre-existing 0755 dir is tightened.
func TestPermsTightensLooseExistingDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := openAt(t, filepath.Join(dir, "tock.db"))
	defer s.Close()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m := fi.Mode().Perm(); m != 0o700 {
		t.Fatalf("dir mode = %o, want 700", m)
	}
}

func TestPragmasReadBack(t *testing.T) {
	s := openAt(t, dbPath(t))
	defer s.Close()
	var mode string
	var busy, fk int
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q (err %v), want wal", mode, err)
	}
	if err := s.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil || busy != 5000 {
		t.Errorf("busy_timeout = %d (err %v), want 5000", busy, err)
	}
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d (err %v), want 1", fk, err)
	}
}

func TestErrNotWALIsDistinct(t *testing.T) {
	if ErrNotWAL == nil || errors.Is(ErrNotWAL, ErrSchemaTooNew) {
		t.Fatal("ErrNotWAL must be its own sentinel")
	}
}

func TestMigrateRejectsMalformedName(t *testing.T) {
	for _, name := range []string{"1_init.sql", "0001-init.sql", "0001_Init.sql", "0001_.sql", "abcd_init.sql", "0001_init.SQL.sql"} {
		fsys := fstest.MapFS{"migrations/" + name: {Data: []byte(`CREATE TABLE schema_version(version INTEGER, applied_at INTEGER);`)}}
		db := rawMem(t)
		if err := migrateFS(db, fsys, "migrations"); err == nil {
			t.Errorf("name %q accepted, want startup error", name)
		}
	}
}

// TestMigrateFailureRollsBackThatMigration: a broken migration 2 leaves no
// version row and no partial table, and version 1 stays applied.
func TestMigrateFailureRollsBackThatMigration(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/0001_init.sql": {Data: []byte(`CREATE TABLE schema_version(version INTEGER NOT NULL, applied_at INTEGER NOT NULL);`)},
		"migrations/0002_bad.sql":  {Data: []byte(`CREATE TABLE half(x INTEGER); INSERT INTO nope VALUES(1);`)},
	}
	db := rawMem(t)
	if err := migrateFS(db, fsys, "migrations"); err == nil {
		t.Fatal("broken migration accepted")
	}
	if got := versionRows(t, db); len(got) != 1 || got[0] != 1 {
		t.Errorf("schema_version rows = %v, want [1]", got)
	}
	if objectExists(t, db, "table", "half") {
		t.Error("partial table from failed migration survived")
	}
}

func rawMem(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}
