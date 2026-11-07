package store

import (
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"tock/internal/snapshot"
)

// olderDB writes a database at the second-to-last real schema version.
func olderDB(t *testing.T) (path string, from int) {
	t.Helper()
	ms, err := loadMigrations(migrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	sub := fstest.MapFS{}
	for _, m := range ms[:len(ms)-1] {
		b, err := fs.ReadFile(migrationFS, "migrations/"+m.name)
		if err != nil {
			t.Fatal(err)
		}
		sub["m/"+m.name] = &fstest.MapFile{Data: b}
	}
	path = dbPath(t)
	db, err := sql.Open("sqlite", "file:"+path+"?_txlock=immediate&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateFS(db, sub, "m", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO span(id, start, "end", bundle_id, app) VALUES(1, 1000, 2000, 'b', 'A')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	return path, ms[len(ms)-2].version
}

func TestOpenCopiesBeforeMigrating(t *testing.T) {
	path, from := olderDB(t)
	s := openAt(t, path)
	s.Close()
	pre := filepath.Join(AutoDir(path), snapshot.PreName(from, LatestVersion()))
	if _, err := os.Stat(pre); err != nil {
		t.Fatalf("no pre-migration snapshot: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "x.db")
	if err := snapshot.Extract(pre, dst); err != nil {
		t.Fatal(err)
	}
	db, _ := sql.Open("sqlite", "file:"+dst+"?mode=ro")
	defer db.Close()
	if v, _ := currentVersion(db); v != from {
		t.Fatalf("snapshot schema = %d, want %d", v, from)
	}
}

func TestOpenNoCopyForFreshOrCurrent(t *testing.T) {
	path := dbPath(t)
	openAt(t, path).Close()
	openAt(t, path).Close()
	if l, _ := snapshot.List(AutoDir(path)); len(l) != 0 {
		t.Fatalf("snapshots for a fresh/current db: %+v", l)
	}
}

func TestOpenCopyFailureBlocksMigration(t *testing.T) {
	path, from := olderDB(t)
	// A file where the backups dir should be makes the copy fail.
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "backups"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("Open migrated without a pre-migration copy")
	}
	db, _ := sql.Open("sqlite", "file:"+path+"?mode=ro")
	defer db.Close()
	if v, _ := currentVersion(db); v != from {
		t.Fatalf("schema = %d after failed copy, want untouched %d", v, from)
	}
}
