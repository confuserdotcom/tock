// Package store owns the SQLite database. The daemon is the single writer
// (D-22). It imports no cgo package.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"

	"tock/internal/span"
)

// ErrUnknownOp is returned by Apply for an op kind it does not understand.
var ErrUnknownOp = errors.New("store: unknown op kind")

// Store is a handle on tock.db.
type Store struct {
	db *sql.DB
}

// Open creates the data dir (0700) and DB file (0600), opens it in WAL mode
// and applies migrations.
func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: mkdir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: chmod dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("store: create db file: %w", err)
	}
	_ = f.Close()

	dsn := "file:" + path + "?_txlock=immediate&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		_ = db.Close()
		return nil, err
	}
	if mode != "wal" {
		_ = db.Close()
		return nil, fmt.Errorf("store: journal_mode is %q, WAL required", mode)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Apply runs ops in one transaction using parameterized statements only. The
// first failing op rolls back the whole call; the error names its index and kind.
func (s *Store) Apply(ops []span.Op) error {
	if len(ops) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	// Any early return rolls the whole call back; Rollback after Commit is a
	// harmless no-op.
	defer func() { _ = tx.Rollback() }()
	for i, op := range ops {
		var err error
		switch op.Kind {
		case span.OpOpen:
			sp := op.Span
			var title, url any
			if !sp.Private {
				if sp.Title != nil {
					title = *sp.Title
				}
				if sp.URL != nil {
					url = *sp.URL
				}
			}
			priv := 0
			if sp.Private {
				priv = 1
			}
			_, err = tx.Exec(`INSERT INTO span(id,start,"end",bundle_id,app,title,url,private,source) VALUES(?,?,NULL,?,?,?,?,?,'none')`,
				sp.ID, sp.Start, sp.BundleID, sp.App, title, url, priv)
		case span.OpClose, span.OpExtend:
			_, err = tx.Exec(`UPDATE span SET "end"=? WHERE id=?`, op.TS, op.ID)
		case span.OpDelete:
			_, err = tx.Exec(`DELETE FROM span WHERE id=?`, op.ID)
		case span.OpReopen:
			_, err = tx.Exec(`UPDATE span SET "end"=NULL WHERE id=?`, op.ID)
		case span.OpHeartbeat:
			_, err = tx.Exec(`INSERT INTO daemon_state(key,value,updated) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated=excluded.updated`,
				KeyHeartbeat, strconv.FormatInt(op.TS, 10), op.TS)
		default:
			return fmt.Errorf("store: apply op %d (kind %d): %w", i, int(op.Kind), ErrUnknownOp)
		}
		if err != nil {
			return fmt.Errorf("store: apply op %d (%s): %w", i, op.Kind, err)
		}
	}
	return tx.Commit()
}

// Recover closes every span left open by an unclean stop (D-16, TRK-06). The
// end is the last heartbeat, but never earlier than the span's own start, and
// the span's own start when no heartbeat was ever written. It returns how many
// spans it closed; no span has a NULL end afterwards.
func (s *Store) Recover() (int64, error) {
	res, err := s.db.Exec(`UPDATE span SET "end" = MAX(start, COALESCE(
		(SELECT CAST(value AS INTEGER) FROM daemon_state WHERE key = ?), start))
		WHERE "end" IS NULL`, KeyHeartbeat)
	if err != nil {
		return 0, fmt.Errorf("store: recover: %w", err)
	}
	return res.RowsAffected()
}

// NextID returns the next free span id.
func (s *Store) NextID() (int64, error) {
	var n int64
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(id),0)+1 FROM span`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
