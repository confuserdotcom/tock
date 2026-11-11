package store

import (
	"database/sql"
	"fmt"
	"strconv"
)

// CompatLevel is the schema compatibility level this binary writes with.
// It goes up only with a migration that breaks older writers.
const CompatLevel = 1

// KeyTrackdStopped is why trackd stopped and stays down ("" or absent while
// it may run); StoppedSchemaNewer is the one reason so far: the database
// needs a newer trackd.
const (
	KeyTrackdStopped   = "trackd.stopped"
	StoppedSchemaNewer = "schema_newer"
)

// readCompat is the database's compat level: 0 before migration 0025.
func readCompat(db *sql.DB) (int, error) {
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_meta'`).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	var v string
	err := db.QueryRow(`SELECT value FROM schema_meta WHERE key = 'compat'`).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	c, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("store: compat %q: %w", v, err)
	}
	return c, nil
}

// Compat reads the database's compat level. trackd checks it while running:
// a newer tock may raise it under a trackd that is already open.
func (s *Store) Compat() (int, error) {
	c, err := readCompat(s.db)
	if err != nil {
		return 0, fmt.Errorf("store: read compat: %w", err)
	}
	return c, nil
}

// MarkStopped records why trackd stopped in a database it will not open
// (its compat is too high), so tock status can say so. It writes only
// daemon_state, whose shape every migration keeps.
func MarkStopped(path, reason string, nowMS int64) error {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("store: mark stopped: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO daemon_state(key,value,updated) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated=excluded.updated`, KeyTrackdStopped, reason, nowMS); err != nil {
		return fmt.Errorf("store: mark stopped: %w", err)
	}
	return nil
}
