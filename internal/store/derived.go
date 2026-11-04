package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// Derived is one derived result and the key of the inputs it was
// computed from.
type Derived struct {
	Key  string
	Data []byte
}

// DerivedRows returns the rows of names; names with no row are
// absent.
func (s *Store) DerivedRows(names []string) (map[string]Derived, error) {
	out := map[string]Derived{}
	if len(names) == 0 {
		return out, nil
	}
	args := make([]any, len(names))
	for i, d := range names {
		args[i] = d
	}
	rows, err := s.db.Query(`SELECT name, key, data FROM derived WHERE name IN (?`+strings.Repeat(",?", len(names)-1)+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: derived: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var c Derived
		if err := rows.Scan(&name, &c.Key, &c.Data); err != nil {
			return nil, fmt.Errorf("store: derived: %w", err)
		}
		out[name] = c
	}
	return out, rows.Err()
}

// PutDerived stores rows by name, replacing the old ones, in one
// transaction.
func (s *Store) PutDerived(rows map[string]Derived, nowMS int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: put derived: %w", err)
	}
	for name, c := range rows {
		if _, err := tx.Exec(`INSERT INTO derived(name, key, data, at_ms) VALUES(?, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET key = excluded.key, data = excluded.data, at_ms = excluded.at_ms`,
			name, c.Key, c.Data, nowMS); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: put derived %s: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: put derived: %w", err)
	}
	return nil
}

// SpanMark is a label attached to one span, with the span's time: a manual
// override, or the label kept when its title was pruned.
type SpanMark struct {
	SpanID, StartMS, EndMS int64 // EndMS is openMS for the open span
	Kept                   bool
	Category, Project      string
}

// SpanMarks returns the overrides and kept labels of the spans that overlap
// [from, to), the open span ending at openMS, in a stable order.
func (s *Store) SpanMarks(from, to, openMS int64) ([]SpanMark, error) {
	rows, err := s.db.Query(`
		SELECT o.span_id, s.start, coalesce(s."end", ?1), 0, coalesce(o.category, ''), coalesce(o.project, '')
		FROM override o JOIN span s ON s.id = o.span_id
		WHERE s.start < ?3 AND coalesce(s."end", ?1) > ?2
		UNION ALL
		SELECT k.span_id, s.start, coalesce(s."end", ?1), 1, k.category, k.project
		FROM kept k JOIN span s ON s.id = k.span_id
		WHERE s.start < ?3 AND coalesce(s."end", ?1) > ?2
		ORDER BY 1, 4`, openMS, from, to)
	if err != nil {
		return nil, fmt.Errorf("store: span marks: %w", err)
	}
	defer rows.Close()
	var out []SpanMark
	for rows.Next() {
		var m SpanMark
		if err := rows.Scan(&m.SpanID, &m.StartMS, &m.EndMS, &m.Kept, &m.Category, &m.Project); err != nil {
			return nil, fmt.Errorf("store: span marks: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// FirstMS is the start of the earliest span or other machine's view, and
// false when there is neither.
func (s *Store) FirstMS() (int64, bool, error) {
	var ms sql.NullInt64
	err := s.db.QueryRow(`SELECT min(m) FROM (SELECT min(start) AS m FROM span UNION ALL SELECT min(start_ms) FROM remote_view)`).Scan(&ms)
	if err != nil {
		return 0, false, fmt.Errorf("store: first ms: %w", err)
	}
	return ms.Int64, ms.Valid, nil
}
