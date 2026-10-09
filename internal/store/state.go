package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// daemon_state keys. Phase 3's banner reads ax_trusted, automation.* and
// version; heartbeat_ts drives crash recovery.
const (
	// KeyHeartbeat is the daemon_state key holding the last heartbeat timestamp.
	KeyHeartbeat = "heartbeat_ts"
	// KeyAXTrusted holds "0" or "1": whether Accessibility is granted.
	KeyAXTrusted = "ax_trusted"
	// KeyVersion holds the trackd version string.
	KeyVersion = "version"
)

// AutomationKey returns the daemon_state key for the Automation permission of
// one browser bundle id. Its value is "granted", "denied", "unknown" or
// "not_running".
func AutomationKey(bundleID string) string {
	return "automation." + bundleID
}

// SetState upserts one daemon_state row. nowMS is the caller's unix ms and is
// stored in the updated column.
func (s *Store) SetState(key, value string, nowMS int64) error {
	_, err := s.db.Exec(`INSERT INTO daemon_state(key,value,updated) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated=excluded.updated`,
		key, value, nowMS)
	if err != nil {
		return fmt.Errorf("store: set state %q: %w", key, err)
	}
	return nil
}

// State returns the value of one daemon_state key. A missing key returns
// ("", false, nil).
func (s *Store) State(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM daemon_state WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: state %q: %w", key, err)
	}
	return v, true, nil
}

// States returns every daemon_state row as key -> value.
func (s *Store) States() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM daemon_state`)
	if err != nil {
		return nil, fmt.Errorf("store: states: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("store: states: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: states: %w", err)
	}
	return out, nil
}
