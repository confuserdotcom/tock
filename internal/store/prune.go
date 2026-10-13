package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// URLHost cuts a stored URL ("host/path?query") to its host.
func URLHost(u string) string {
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		return u[:i]
	}
	return u
}

// titled is the condition for a span or remote view that still holds a
// title or a URL path.
const titled = `(title IS NOT NULL OR (url IS NOT NULL AND (instr(url, '/') > 0 OR instr(url, '?') > 0 OR instr(url, '#') > 0)))`

// TitledBefore returns the closed spans that ended at or before beforeMS
// and still hold a title or a URL path, ordered by start.
func (s *Store) TitledBefore(beforeMS int64) ([]SpanRow, error) {
	rows, err := s.db.Query(`SELECT id, start, "end", bundle_id, app, title, url, private
		FROM span WHERE "end" IS NOT NULL AND "end" <= ? AND `+titled+` ORDER BY start, id`, beforeMS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpanRow
	for rows.Next() {
		var r SpanRow
		var end int64
		var title, url sql.NullString
		var private int
		if err := rows.Scan(&r.ID, &r.Start, &end, &r.BundleID, &r.App, &title, &url, &private); err != nil {
			return nil, err
		}
		r.End, r.Private = &end, private == 1
		if title.Valid {
			r.Title = &title.String
		}
		if url.Valid {
			r.URL = &url.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneSpans deletes the titles of the spans in kept, cuts their URLs to
// the host and saves each one's kept label, in one transaction. A span
// that already has a kept label keeps it.
func (s *Store) PruneSpans(kept []Kept) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, k := range kept {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO kept(span_id, category, project) VALUES(?, ?, ?)`,
			k.SpanID, k.Category, k.Project); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: keep span %d: %w", k.SpanID, err)
		}
		var url sql.NullString
		if err := tx.QueryRow(`SELECT url FROM span WHERE id = ?`, k.SpanID).Scan(&url); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: prune span %d: %w", k.SpanID, err)
		}
		var host any
		if url.Valid {
			host = URLHost(url.String)
		}
		if _, err := tx.Exec(`UPDATE span SET title = NULL, url = ? WHERE id = ?`, host, k.SpanID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: prune span %d: %w", k.SpanID, err)
		}
	}
	return tx.Commit()
}

// PruneRemote deletes the titles and URL paths of other machines' views
// that ended at or before beforeMS. It returns how many it changed.
func (s *Store) PruneRemote(beforeMS int64) (int, error) {
	rows, err := s.db.Query(`SELECT id, coalesce(url, '') FROM remote_view WHERE end_ms <= ? AND `+titled, beforeMS)
	if err != nil {
		return 0, err
	}
	type item struct {
		id  int64
		url string
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.url); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	for _, it := range items {
		if _, err := tx.Exec(`UPDATE remote_view SET title = NULL, url = nullif(?, '') WHERE id = ?`, URLHost(it.url), it.id); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
	}
	return len(items), tx.Commit()
}
