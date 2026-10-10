package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// SetPlan saves the plan of the week starting on Monday week (YYYY-MM-DD),
// replacing any earlier one.
func (s *Store) SetPlan(week, text string, atMS int64) error {
	_, err := s.db.Exec(`INSERT INTO plan(week, text, at_ms) VALUES(?, ?, ?)
		ON CONFLICT(week) DO UPDATE SET text = excluded.text, at_ms = excluded.at_ms`, week, text, atMS)
	if err != nil {
		return fmt.Errorf("store: set plan: %w", err)
	}
	return nil
}

// Plan returns the plan of a week, "" when there is none.
func (s *Store) Plan(week string) (string, error) {
	var text string
	err := s.db.QueryRow(`SELECT text FROM plan WHERE week = ?`, week).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return text, err
}
