package store

import "fmt"

// SaveSummary replaces the day's summaries: the day's text under start 0
// and each block's under its start.
func (s *Store) SaveSummary(day, model string, dayText string, blocks map[int64]string, nowMS int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM summary WHERE day = ?`, day); err != nil {
		return fmt.Errorf("store: save summary: %w", err)
	}
	put := func(start int64, text string) error {
		_, err := tx.Exec(`INSERT INTO summary(day, start_ms, text, model, created_ms) VALUES(?, ?, ?, ?, ?)`,
			day, start, text, model, nowMS)
		return err
	}
	if err := put(0, dayText); err != nil {
		return fmt.Errorf("store: save summary: %w", err)
	}
	for start, text := range blocks {
		if err := put(start, text); err != nil {
			return fmt.Errorf("store: save summary: %w", err)
		}
	}
	return tx.Commit()
}

// Summaries returns the day's summaries by start (0 = the day) and when
// they were written; an empty map when there are none.
func (s *Store) Summaries(day string) (map[int64]string, int64, error) {
	rows, err := s.db.Query(`SELECT start_ms, text, created_ms FROM summary WHERE day = ?`, day)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := map[int64]string{}
	var at int64
	for rows.Next() {
		var start, created int64
		var text string
		if err := rows.Scan(&start, &text, &created); err != nil {
			return nil, 0, err
		}
		out[start], at = text, max(at, created)
	}
	return out, at, rows.Err()
}
