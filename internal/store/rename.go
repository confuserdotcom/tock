package store

import "fmt"

// RenameLabel renames a category or project ("category" or "project") from
// one name to another, case-insensitively, in manual overrides, away labels
// and cached suggestions. Renamed away rows get atMS so caches see the change.
func (s *Store) RenameLabel(column, from, to string, atMS int64) error {
	if column != "category" && column != "project" {
		return fmt.Errorf("store: rename label: column %q", column)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE override SET ` + column + ` = ? WHERE lower(` + column + `) = lower(?)`, []any{to, from}},
		{`UPDATE away SET ` + column + ` = ?, at_ms = ? WHERE lower(` + column + `) = lower(?)`, []any{to, atMS, from}},
		{`UPDATE suggestion SET ` + column + ` = ? WHERE lower(` + column + `) = lower(?)`, []any{to, from}},
	} {
		if _, err := tx.Exec(q.sql, q.args...); err != nil {
			return fmt.Errorf("store: rename label: %w", err)
		}
	}
	return tx.Commit()
}
