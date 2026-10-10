package store

import (
	"errors"
	"fmt"
)

// ErrNoAway is returned when an away row does not exist.
var ErrNoAway = errors.New("store: no such away period")

// Away is a labelled (or skipped, with no category and no project) absence.
type Away struct {
	ID                int64
	StartMS, EndMS    int64
	Category, Project string
	AtMS              int64
}

// AddAway inserts a and returns its id.
func (s *Store) AddAway(a Away) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO away(start_ms, end_ms, category, project, at_ms) VALUES(?, ?, ?, ?, ?)`,
		a.StartMS, a.EndMS, a.Category, a.Project, a.AtMS)
	if err != nil {
		return 0, fmt.Errorf("store: add away: %w", err)
	}
	return res.LastInsertId()
}

// Aways returns the rows overlapping [from, to), in start order.
func (s *Store) Aways(from, to int64) ([]Away, error) {
	rows, err := s.db.Query(`SELECT id, start_ms, end_ms, category, project, at_ms FROM away
		WHERE end_ms > ? AND start_ms < ? ORDER BY start_ms, id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Away
	for rows.Next() {
		var a Away
		if err := rows.Scan(&a.ID, &a.StartMS, &a.EndMS, &a.Category, &a.Project, &a.AtMS); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetAwayLabel relabels row id. An empty field keeps the stored value.
func (s *Store) SetAwayLabel(id int64, category, project string, atMS int64) error {
	res, err := s.db.Exec(`UPDATE away SET
		category = CASE WHEN ? = '' THEN category ELSE ? END,
		project  = CASE WHEN ? = '' THEN project ELSE ? END,
		at_ms = ? WHERE id = ?`, category, category, project, project, atMS, id)
	if err != nil {
		return fmt.Errorf("store: set away label: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNoAway
	}
	return nil
}

// AwayStamp changes whenever an away row is added or relabelled: the row
// count and the latest write time.
func (s *Store) AwayStamp() (n, lastAt int64, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(at_ms), 0) FROM away`).Scan(&n, &lastAt)
	return n, lastAt, err
}
