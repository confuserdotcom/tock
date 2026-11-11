package store

import (
	"database/sql"
	"fmt"
	"math"
)

// SpanRow is one span as stored. End is nil for the open span.
type SpanRow struct {
	ID, Start  int64
	End        *int64
	BundleID   string
	App        string
	Title, URL *string
	Private    bool
	Kept       *Kept // the label kept when the title was deleted; nil = none
}

// Kept is the rule label a span had when its title was deleted.
type Kept struct {
	SpanID            int64
	Category, Project string
}

// spansQuery reads the spans overlapping [from, to). Its arguments are to,
// the lower bound on start from spanLow, and from. Without the lower bound
// the span_start index bounds the scan only from above, and every read walks
// all history before the range.
const spansQuery = `SELECT id, start, "end", bundle_id, app, title, url, private,
		kept.span_id IS NOT NULL, coalesce(kept.category, ''), coalesce(kept.project, '')
		FROM span LEFT JOIN kept ON kept.span_id = span.id
		WHERE start < ? AND start >= ? AND ("end" IS NULL OR "end" > ?) ORDER BY start, id`

// spanLow is the earliest start a span overlapping [from, …) can have: from
// minus the longest closed span (span_len makes that one index lookup), or
// the start of the oldest open span if that is earlier.
func (s *Store) spanLow(from int64) (int64, error) { return spanLowTx(s.db, from) }

// back is from minus d, saturating at math.MinInt64 (AllSpans reads from
// there) and ignoring a negative d.
func back(from, d int64) int64 {
	if d <= 0 {
		return from
	}
	if from < math.MinInt64+d {
		return math.MinInt64
	}
	return from - d
}

func spanLowTx(q querier, from int64) (int64, error) {
	var longest int64
	var open sql.NullInt64
	if err := q.QueryRow(`SELECT coalesce((SELECT max("end" - start) FROM span), 0),
		(SELECT min(start) FROM span WHERE "end" IS NULL)`).Scan(&longest, &open); err != nil {
		return 0, fmt.Errorf("store: span bound: %w", err)
	}
	lo := back(from, longest)
	if open.Valid && open.Int64 < lo {
		lo = open.Int64
	}
	return lo, nil
}

// Spans returns the spans overlapping [from, to), ordered by start.
func (s *Store) Spans(from, to int64) ([]SpanRow, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("store: spans: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // read only
	lo, err := spanLowTx(tx, from)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(spansQuery, to, lo, from)
	if err != nil {
		return nil, fmt.Errorf("store: spans: %w", err)
	}
	defer rows.Close()
	var out []SpanRow
	for rows.Next() {
		var r SpanRow
		var end sql.NullInt64
		var title, url sql.NullString
		var private int
		var kept bool
		var k Kept
		if err := rows.Scan(&r.ID, &r.Start, &end, &r.BundleID, &r.App, &title, &url, &private,
			&kept, &k.Category, &k.Project); err != nil {
			return nil, err
		}
		if kept {
			k.SpanID = r.ID
			r.Kept = &k
		}
		if end.Valid {
			r.End = &end.Int64
		}
		if title.Valid {
			r.Title = &title.String
		}
		if url.Valid {
			r.URL = &url.String
		}
		r.Private = private == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// AllSpans returns every span, ordered by start.
func (s *Store) AllSpans() ([]SpanRow, error) {
	return s.Spans(math.MinInt64, math.MaxInt64)
}

// Override is a manual label for one span. Empty fields are unset.
type Override struct {
	SpanID            int64
	Category, Project string
	At                int64
}

// Overrides returns all manual labels keyed by span id.
func (s *Store) Overrides() (map[int64]Override, error) {
	rows, err := s.db.Query(`SELECT span_id, coalesce(category, ''), coalesce(project, ''), at FROM override`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Override{}
	for rows.Next() {
		var o Override
		if err := rows.Scan(&o.SpanID, &o.Category, &o.Project, &o.At); err != nil {
			return nil, err
		}
		out[o.SpanID] = o
	}
	return out, rows.Err()
}

// SetOverrides upserts manual labels in one transaction. A field left empty
// keeps the span's existing manual value for that field.
func (s *Store) SetOverrides(os []Override) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, o := range os {
		if _, err := tx.Exec(`INSERT INTO override(span_id, category, project, at)
			VALUES(?, NULLIF(?, ''), NULLIF(?, ''), ?)
			ON CONFLICT(span_id) DO UPDATE SET
			  category = coalesce(excluded.category, override.category),
			  project  = coalesce(excluded.project, override.project),
			  at       = excluded.at`, o.SpanID, o.Category, o.Project, o.At); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: override span %d: %w", o.SpanID, err)
		}
	}
	return tx.Commit()
}

// Ignored returns the inbox keys the user hid.
func (s *Store) Ignored() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT key FROM inbox_ignore`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

// SetIgnored hides (on) or shows again (off) an inbox key.
func (s *Store) SetIgnored(key string, on bool, nowMS int64) error {
	var err error
	if on {
		_, err = s.db.Exec(`INSERT INTO inbox_ignore(key, at) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET at = excluded.at`, key, nowMS)
	} else {
		_, err = s.db.Exec(`DELETE FROM inbox_ignore WHERE key = ?`, key)
	}
	return err
}
