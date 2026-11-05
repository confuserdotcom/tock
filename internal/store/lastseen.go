package store

import (
	"database/sql"
	"errors"
	"fmt"

	"tock/internal/span"
)

// LastSeen returns the latest sign of a running trackd: the larger of the
// last heartbeat and the newest span start, in unix ms (0 when trackd never
// wrote anything). The heartbeat advances every 10 s while a span is open;
// the span start covers the seconds after unlock or wake before the first
// heartbeat.
func (s *Store) LastSeen() (int64, error) {
	var ms int64
	err := s.db.QueryRow(`SELECT max(
		coalesce((SELECT CAST(value AS INTEGER) FROM daemon_state WHERE key = ?), 0),
		coalesce((SELECT max(start) FROM span), 0))`, KeyHeartbeat).Scan(&ms)
	return ms, err
}

// Active is the app, and its URL (host+path, nil = none), of a span.
type Active struct {
	Bundle, App string
	URL         *string
}

// LastActive is the newest span that is not idle, or nil.
func (s *Store) LastActive() (*Active, error) {
	var a Active
	var url sql.NullString
	err := s.db.QueryRow(`SELECT bundle_id, app, url FROM span WHERE bundle_id != ?
		ORDER BY start DESC, id DESC LIMIT 1`, span.IdleBundleID).Scan(&a.Bundle, &a.App, &url)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: last active: %w", err)
	}
	if url.Valid {
		a.URL = &url.String
	}
	return &a, nil
}

// AppName is the newest app name recorded for bundle, or "".
func (s *Store) AppName(bundle string) (string, error) {
	var n string
	err := s.db.QueryRow(`SELECT app FROM span WHERE bundle_id = ? ORDER BY start DESC LIMIT 1`, bundle).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: app name: %w", err)
	}
	return n, nil
}
