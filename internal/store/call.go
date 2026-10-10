package store

import "fmt"

// CallGapMS is the longest pause inside one call: a later tick starts a new
// row.
const CallGapMS = 30_000

// Call is one stretch of camera or microphone use.
type Call struct{ StartMS, EndMS int64 }

// RecordCall notes that a call is going on at now: it extends the latest
// call that ended within CallGapMS before now, or starts a new one. There is
// no open state, so a crash loses at most one tick.
func (s *Store) RecordCall(now int64) error {
	res, err := s.db.Exec(`UPDATE call SET end_ms = MAX(end_ms, ?) WHERE id = (
		SELECT id FROM call WHERE end_ms >= ? ORDER BY end_ms DESC LIMIT 1)`, now, now-CallGapMS)
	if err != nil {
		return fmt.Errorf("store: record call: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO call(start_ms, end_ms) VALUES(?, ?)`, now, now); err != nil {
		return fmt.Errorf("store: record call: %w", err)
	}
	return nil
}

// Calls returns the calls overlapping [from, to), in start order.
func (s *Store) Calls(from, to int64) ([]Call, error) {
	rows, err := s.db.Query(`SELECT start_ms, end_ms FROM call WHERE end_ms > ? AND start_ms < ? ORDER BY start_ms`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Call
	for rows.Next() {
		var c Call
		if err := rows.Scan(&c.StartMS, &c.EndMS); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
