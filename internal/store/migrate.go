package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ErrSchemaTooNew is returned when the database was written by a newer trackd.
var ErrSchemaTooNew = errors.New("store: database schema is newer than this trackd")

// migrationName is the only accepted migration file name shape.
var migrationName = regexp.MustCompile(`^([0-9]{4})_[a-z0-9_]+\.sql$`)

func currentVersion(db *sql.DB) (int, error) {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_version'`).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	var v sql.NullInt64
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&v); err != nil {
		return 0, err
	}
	return int(v.Int64), nil
}

type migration struct {
	version int
	name    string
}

// loadMigrations lists the migrations in dir sorted by version. A file whose
// name does not match migrationName, or a duplicate version, is an error.
func loadMigrations(fsys fs.FS, dir string) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var ms []migration
	seen := map[int]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		m := migrationName.FindStringSubmatch(name)
		if m == nil {
			return nil, fmt.Errorf("store: bad migration name %q (want NNNN_name.sql)", name)
		}
		v, _ := strconv.Atoi(m[1])
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("store: migrations %q and %q share version %d", prev, name, v)
		}
		seen[v] = name
		ms = append(ms, migration{version: v, name: name})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	return ms, nil
}

func migrate(db *sql.DB) error {
	return migrateFS(db, migrationFS, "migrations")
}

// migrateFS applies the migrations in fsys/dir. The refuse-newer check only
// reads, so a too-new database is never written to (D-23). Each migration runs
// in its own transaction together with its schema_version row.
func migrateFS(db *sql.DB, fsys fs.FS, dir string) error {
	ms, err := loadMigrations(fsys, dir)
	if err != nil {
		return err
	}
	cur, err := currentVersion(db)
	if err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	latest := 0
	if len(ms) > 0 {
		latest = ms[len(ms)-1].version
	}
	if cur > latest {
		return fmt.Errorf("%w (database v%d, this binary v%d)", ErrSchemaTooNew, cur, latest)
	}

	for _, m := range ms {
		if m.version <= cur {
			continue
		}
		body, err := fs.ReadFile(fsys, dir+"/"+m.name)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: migration %s: %w", m.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_version(version, applied_at) VALUES(?, ?)`,
			m.version, time.Now().Round(0).UnixMilli()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: record migration %s: %w", m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit migration %s: %w", m.name, err)
		}
	}
	return nil
}
