package store

import (
	"fmt"
	"strings"
)

// CachedDay is one closed day's derived result and the key of the inputs
// it was computed from.
type CachedDay struct {
	Key  string
	Data []byte
}

// DayCache returns the cached rows of days, by date; days with no row are
// absent.
func (s *Store) DayCache(days []string) (map[string]CachedDay, error) {
	out := map[string]CachedDay{}
	if len(days) == 0 {
		return out, nil
	}
	args := make([]any, len(days))
	for i, d := range days {
		args[i] = d
	}
	rows, err := s.db.Query(`SELECT day, key, data FROM day_cache WHERE day IN (?`+strings.Repeat(",?", len(days)-1)+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: day cache: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var day string
		var c CachedDay
		if err := rows.Scan(&day, &c.Key, &c.Data); err != nil {
			return nil, fmt.Errorf("store: day cache: %w", err)
		}
		out[day] = c
	}
	return out, rows.Err()
}

// PutDayCache stores days, replacing their rows, in one transaction.
func (s *Store) PutDayCache(days map[string]CachedDay, nowMS int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: put day cache: %w", err)
	}
	for day, c := range days {
		if _, err := tx.Exec(`INSERT INTO day_cache(day, key, data, at_ms) VALUES(?, ?, ?, ?)
			ON CONFLICT(day) DO UPDATE SET key = excluded.key, data = excluded.data, at_ms = excluded.at_ms`,
			day, c.Key, c.Data, nowMS); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: put day cache %s: %w", day, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: put day cache: %w", err)
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
