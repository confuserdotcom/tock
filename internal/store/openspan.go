package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// OpenBundle returns the bundle id of the span trackd has open, if any.
func (s *Store) OpenBundle() (string, bool, error) {
	var b string
	err := s.db.QueryRow(`SELECT bundle_id FROM span WHERE "end" IS NULL ORDER BY start DESC LIMIT 1`).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: open span: %w", err)
	}
	return b, true, nil
}
