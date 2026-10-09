package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// LatestSchemaVersion is the newest schema this binary knows.
const LatestSchemaVersion = 1

// ErrSchemaTooNew is returned when the database was written by a newer trackd.
var ErrSchemaTooNew = errors.New("store: database schema is newer than this trackd")

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

func migrate(db *sql.DB) error {
	cur, err := currentVersion(db)
	if err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	if cur > LatestSchemaVersion {
		return ErrSchemaTooNew
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	var ms []migration
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") || len(name) < 5 {
			continue
		}
		v, err := strconv.Atoi(name[:4])
		if err != nil {
			return fmt.Errorf("store: bad migration name %q", name)
		}
		ms = append(ms, migration{version: v, name: name})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })

	for _, m := range ms {
		if m.version <= cur {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + m.name)
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
			return err
		}
	}
	return nil
}
