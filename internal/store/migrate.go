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

func migrate(db *sql.DB, before func(from, to int) error) error {
	return migrateFS(db, migrationFS, "migrations", before)
}

// LatestVersion is the schema version this binary migrates to.
func LatestVersion() int {
	ms, err := loadMigrations(migrationFS, "migrations")
	if err != nil || len(ms) == 0 {
		return 0
	}
	return ms[len(ms)-1].version
}

// migrateFS applies the migrations in fsys/dir. The refuse-newer check only
// reads, so a too-new database is never written to (D-23). before, when not
// nil, runs once before the first migration of an existing database (cur >
// 0); an error from it stops the migration.
func migrateFS(db *sql.DB, fsys fs.FS, dir string, before func(from, to int) error) error {
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
	// A newer tock's additive migrations leave compat alone, and this
	// binary goes on with the tables it knows (ADR 0002).
	c, err := readCompat(db)
	if err != nil {
		return fmt.Errorf("store: read compat: %w", err)
	}
	if c > CompatLevel {
		return fmt.Errorf("%w (database v%d compat %d, this binary v%d compat %d)", ErrSchemaTooNew, cur, c, latest, CompatLevel)
	}
	if cur > latest {
		return nil
	}
	if cur == latest {
		return nil
	}
	if cur > 0 && before != nil {
		if err := before(cur, latest); err != nil {
			return fmt.Errorf("store: pre-migration backup: %w", err)
		}
	}
	return applyMigrations(db, fsys, dir, ms, cur)
}

// applyMigrations runs each migration above cur in its own immediate
// transaction with its schema_version row. Inside the transaction it checks
// again whether the version is applied: another process may have migrated
// since cur was read.
func applyMigrations(db *sql.DB, fsys fs.FS, dir string, ms []migration, cur int) error {
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
		done, err := appliedTx(tx, m.version)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: migration %s: %w", m.name, err)
		}
		if done {
			_ = tx.Rollback()
			continue
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

// appliedTx reports whether version v is recorded, inside tx.
func appliedTx(tx *sql.Tx, v int) (bool, error) {
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_version'`).Scan(&n); err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	if err := tx.QueryRow(`SELECT count(*) FROM schema_version WHERE version = ?`, v).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
