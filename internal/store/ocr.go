package store

import (
	"fmt"
	"strings"
)

// OCRStatKeepDays is how many days of ocr_stat rows are kept.
const OCRStatKeepDays = 31

// OCRStat is one day's count for one reason: reads done ("ok", with their
// summed cost) or skipped.
type OCRStat struct {
	N      int
	CostMS int64
}

// SetSpanWords stores the screen keywords of span id, replacing any; no
// words deletes the row.
func (s *Store) SetSpanWords(id int64, words []string, atMS int64) error {
	var err error
	if len(words) == 0 {
		_, err = s.db.Exec(`DELETE FROM span_ocr WHERE span_id = ?`, id)
	} else {
		_, err = s.db.Exec(`INSERT INTO span_ocr(span_id, words, at_ms) VALUES(?, ?, ?)
			ON CONFLICT(span_id) DO UPDATE SET words = excluded.words, at_ms = excluded.at_ms`,
			id, strings.Join(words, " "), atMS)
	}
	if err != nil {
		return fmt.Errorf("store: set span words %d: %w", id, err)
	}
	return nil
}

// SpanWords returns the screen keywords of the spans with these ids that
// have some.
func (s *Store) SpanWords(ids []int64) (map[int64][]string, error) {
	out := map[int64][]string{}
	for len(ids) > 0 {
		n := min(len(ids), 500)
		args := make([]any, n)
		for i, id := range ids[:n] {
			args[i] = id
		}
		rows, err := s.db.Query(`SELECT span_id, words FROM span_ocr WHERE span_id IN (?`+strings.Repeat(",?", n-1)+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("store: span words: %w", err)
		}
		for rows.Next() {
			var id int64
			var w string
			if err := rows.Scan(&id, &w); err != nil {
				rows.Close()
				return nil, fmt.Errorf("store: span words: %w", err)
			}
			out[id] = strings.Fields(w)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("store: span words: %w", err)
		}
		ids = ids[n:]
	}
	return out, nil
}

// PruneSpanWords deletes the screen keywords of spans that ended before
// beforeMS, and of spans that no longer exist. It returns how many it
// deleted.
func (s *Store) PruneSpanWords(beforeMS int64) (int, error) {
	r, err := s.db.Exec(`DELETE FROM span_ocr WHERE span_id NOT IN (SELECT id FROM span WHERE "end" IS NULL OR "end" >= ?)`, beforeMS)
	if err != nil {
		return 0, fmt.Errorf("store: prune span words: %w", err)
	}
	n, err := r.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: prune span words: %w", err)
	}
	return int(n), nil
}

// CountOCR adds one read (reason "ok", costMS its cost) or one skip to day
// ("2006-01-02", local).
func (s *Store) CountOCR(day, reason string, costMS int64) error {
	if _, err := s.db.Exec(`INSERT INTO ocr_stat(day, reason, n, cost_ms) VALUES(?, ?, 1, ?)
		ON CONFLICT(day, reason) DO UPDATE SET n = n + 1, cost_ms = cost_ms + excluded.cost_ms`, day, reason, costMS); err != nil {
		return fmt.Errorf("store: count ocr: %w", err)
	}
	return nil
}

// PruneOCRStats deletes the ocr_stat rows of days before day.
func (s *Store) PruneOCRStats(day string) error {
	if _, err := s.db.Exec(`DELETE FROM ocr_stat WHERE day < ?`, day); err != nil {
		return fmt.Errorf("store: prune ocr stats: %w", err)
	}
	return nil
}

// OCRStats returns day's counts by reason.
func (s *Store) OCRStats(day string) (map[string]OCRStat, error) {
	rows, err := s.db.Query(`SELECT reason, n, cost_ms FROM ocr_stat WHERE day = ?`, day)
	if err != nil {
		return nil, fmt.Errorf("store: ocr stats: %w", err)
	}
	defer rows.Close()
	out := map[string]OCRStat{}
	for rows.Next() {
		var r string
		var st OCRStat
		if err := rows.Scan(&r, &st.N, &st.CostMS); err != nil {
			return nil, fmt.Errorf("store: ocr stats: %w", err)
		}
		out[r] = st
	}
	return out, rows.Err()
}
