package store

import "fmt"

// AutoLabel is one key tock labelled on its own from a confident guess.
type AutoLabel struct {
	Key, Category, Project, Reason, Rule string
	Share                                float64
	MS, AtMS                             int64 // MS: the key's inbox time when labelled
}

// AddAutoLabel records x; a key already recorded keeps its first row.
func (s *Store) AddAutoLabel(x AutoLabel) error {
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO auto_label(key, category, project, reason, share, rule, ms, at_ms)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, x.Key, x.Category, x.Project, x.Reason, x.Share, x.Rule, x.MS, x.AtMS); err != nil {
		return fmt.Errorf("store: add auto label %q: %w", x.Key, err)
	}
	return nil
}

// AutoLabelled reports which of keys were ever auto-labelled.
func (s *Store) AutoLabelled() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT key FROM auto_label`)
	if err != nil {
		return nil, fmt.Errorf("store: auto labelled: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("store: auto labelled: %w", err)
		}
		out[k] = true
	}
	return out, rows.Err()
}

// AutoLabels returns the keys auto-labelled at or after sinceMS, newest
// first.
func (s *Store) AutoLabels(sinceMS int64) ([]AutoLabel, error) {
	rows, err := s.db.Query(`SELECT key, category, project, reason, share, rule, ms, at_ms FROM auto_label
		WHERE at_ms >= ? ORDER BY at_ms DESC, key`, sinceMS)
	if err != nil {
		return nil, fmt.Errorf("store: auto labels: %w", err)
	}
	defer rows.Close()
	var out []AutoLabel
	for rows.Next() {
		var x AutoLabel
		if err := rows.Scan(&x.Key, &x.Category, &x.Project, &x.Reason, &x.Share, &x.Rule, &x.MS, &x.AtMS); err != nil {
			return nil, fmt.Errorf("store: auto labels: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
