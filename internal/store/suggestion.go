package store

import (
	"database/sql"
	"fmt"
	"strconv"
)

// KeyLLMClaim is the daemon_state key holding the unix ms of the last
// suggestion batch attempt.
const KeyLLMClaim = "llm_claim_ms"

// Suggestion is a cached Claude answer for one inbox key. Empty Category and
// Project mean Claude had no suggestion.
type Suggestion struct {
	Key, Category, Project, Model string
	CreatedMS                     int64
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Suggestions returns every cached suggestion by key.
func (s *Store) Suggestions() (map[string]Suggestion, error) {
	rows, err := s.db.Query(`SELECT key, category, project, model, created_ms FROM suggestion`)
	if err != nil {
		return nil, fmt.Errorf("store: suggestions: %w", err)
	}
	defer rows.Close()
	out := map[string]Suggestion{}
	for rows.Next() {
		var x Suggestion
		var cat, proj sql.NullString
		if err := rows.Scan(&x.Key, &cat, &proj, &x.Model, &x.CreatedMS); err != nil {
			return nil, fmt.Errorf("store: suggestions: %w", err)
		}
		x.Category, x.Project = cat.String, proj.String
		out[x.Key] = x
	}
	return out, rows.Err()
}

// SaveSuggestions upserts xs in one transaction.
func (s *Store) SaveSuggestions(xs []Suggestion) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, x := range xs {
		if _, err := tx.Exec(`INSERT INTO suggestion(key, category, project, model, created_ms) VALUES(?,?,?,?,?)
			ON CONFLICT(key) DO UPDATE SET category = excluded.category, project = excluded.project,
			model = excluded.model, created_ms = excluded.created_ms`,
			x.Key, nullStr(x.Category), nullStr(x.Project), x.Model, x.CreatedMS); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: save suggestion: %w", err)
		}
	}
	return tx.Commit()
}

// Claim sets the daemon_state key to nowMS when it is missing or holds a
// time at most nowMS-everyMS, in one statement, and reports whether it did.
// Two tock processes racing for the same hour get one true between them.
func (s *Store) Claim(key string, nowMS, everyMS int64) (bool, error) {
	res, err := s.db.Exec(`INSERT INTO daemon_state(key, value, updated) VALUES(?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated = excluded.updated
		WHERE CAST(daemon_state.value AS INTEGER) <= ?`,
		key, strconv.FormatInt(nowMS, 10), nowMS, nowMS-everyMS)
	if err != nil {
		return false, fmt.Errorf("store: claim %q: %w", key, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: claim %q: %w", key, err)
	}
	return n == 1, nil
}
