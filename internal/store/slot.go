package store

import (
	"fmt"
	"strings"
)

// SlotNoticeKeepMS is how long slot_notice rows are kept.
const SlotNoticeKeepMS = 7 * 24 * 3600_000

// SlotNotice is one slot notice already sent: Key is the slot's key, Kind
// the notice ("upcoming" or "start").
type SlotNotice struct {
	Key, Kind string
	AtMS      int64
}

// MarkSlotNotice records that the kind notice of slot key was sent at atMS
// and reports whether the row is new. Only the caller that gets true sends
// the notice, so a slot notifies once across restarts and racing writers.
// Rows older than SlotNoticeKeepMS are deleted in the same transaction.
func (s *Store) MarkSlotNotice(key, kind string, atMS int64) (inserted bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("store: mark slot notice: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM slot_notice WHERE at_ms < ?`, atMS-SlotNoticeKeepMS); err != nil {
		return false, fmt.Errorf("store: mark slot notice: %w", err)
	}
	r, err := tx.Exec(`INSERT OR IGNORE INTO slot_notice(slot_key, kind, at_ms) VALUES(?, ?, ?)`, key, kind, atMS)
	if err != nil {
		return false, fmt.Errorf("store: mark slot notice: %w", err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: mark slot notice: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: mark slot notice: %w", err)
	}
	return n == 1, nil
}

// SlotNotices returns the notices already sent for the slots with these
// keys; nil for no keys.
func (s *Store) SlotNotices(keys []string) ([]SlotNotice, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	rows, err := s.db.Query(`SELECT slot_key, kind, at_ms FROM slot_notice WHERE slot_key IN (?`+
		strings.Repeat(",?", len(keys)-1)+`) ORDER BY slot_key, kind`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: slot notices: %w", err)
	}
	defer rows.Close()
	var out []SlotNotice
	for rows.Next() {
		var n SlotNotice
		if err := rows.Scan(&n.Key, &n.Kind, &n.AtMS); err != nil {
			return nil, fmt.Errorf("store: slot notices: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnmarkSlotNotice deletes the row MarkSlotNotice wrote for the kind notice
// of slot key, so that notice is sent again: for a notice that was marked but
// could not be handed over. A missing row is not an error.
func (s *Store) UnmarkSlotNotice(key, kind string) error {
	if _, err := s.db.Exec(`DELETE FROM slot_notice WHERE slot_key = ? AND kind = ?`, key, kind); err != nil {
		return fmt.Errorf("store: unmark slot notice: %w", err)
	}
	return nil
}
