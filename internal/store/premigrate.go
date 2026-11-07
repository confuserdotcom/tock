package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	"tock/internal/snapshot"
)

// preFreshMS: a pre-migration snapshot younger than this is the one another
// process just took for the same upgrade, so it is kept as is.
const preFreshMS = 3600 * 1000

// AutoDir is where automatic snapshots of the database at dbPath live.
func AutoDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "backups", "auto")
}

// preMigration returns the hook migrateFS runs before migrating an
// existing database: a checked, compressed copy in AutoDir.
func preMigration(db *sql.DB, path string) func(from, to int) error {
	return func(from, to int) error {
		dir := AutoDir(path)
		name := snapshot.PreName(from, to)
		if fi, err := os.Stat(filepath.Join(dir, name)); err == nil && time.Since(fi.ModTime()) < preFreshMS*time.Millisecond {
			return nil
		}
		var live int64
		if fi, err := os.Stat(path); err == nil {
			live = fi.Size()
		}
		_, err := snapshot.Write(snapshot.Opts{
			Dir: dir, Name: name, LiveSize: live, Free: snapshot.FreeDisk,
			Vacuum: func(p string) error { _, err := db.Exec(`VACUUM INTO ?`, p); return err },
		})
		return err
	}
}
