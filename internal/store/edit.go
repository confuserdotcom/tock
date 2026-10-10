package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrNoEdit is returned when an edit row does not exist.
var ErrNoEdit = errors.New("store: no such edit")

// Edit is one timeline edit.
type Edit struct {
	ID                      int64
	Kind                    string // add, cut or label
	StartMS, EndMS          int64
	Category, Project, Note string
	AtMS                    int64
}

const editCols = `id, kind, start_ms, end_ms, category, project, note, at_ms`

func scanEdit(sc interface{ Scan(...any) error }) (Edit, error) {
	var e Edit
	err := sc.Scan(&e.ID, &e.Kind, &e.StartMS, &e.EndMS, &e.Category, &e.Project, &e.Note, &e.AtMS)
	return e, err
}

// AddEdit inserts e and returns its id.
func (s *Store) AddEdit(e Edit) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO edit(kind, start_ms, end_ms, category, project, note, at_ms) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		e.Kind, e.StartMS, e.EndMS, e.Category, e.Project, e.Note, e.AtMS)
	if err != nil {
		return 0, fmt.Errorf("store: add edit: %w", err)
	}
	return res.LastInsertId()
}

// Edits returns the edits overlapping [from, to) in id order, the order
// they apply in.
func (s *Store) Edits(from, to int64) ([]Edit, error) {
	rows, err := s.db.Query(`SELECT `+editCols+` FROM edit WHERE end_ms > ? AND start_ms < ? ORDER BY id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Edit
	for rows.Next() {
		e, err := scanEdit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EditByID returns one edit.
func (s *Store) EditByID(id int64) (Edit, error) {
	e, err := scanEdit(s.db.QueryRow(`SELECT `+editCols+` FROM edit WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Edit{}, ErrNoEdit
	}
	return e, err
}

// PutEdit writes e back over its row (relabel, or undo of a relabel).
func (s *Store) PutEdit(e Edit) error {
	res, err := s.db.Exec(`UPDATE edit SET kind = ?, start_ms = ?, end_ms = ?, category = ?, project = ?, note = ?, at_ms = ? WHERE id = ?`,
		e.Kind, e.StartMS, e.EndMS, e.Category, e.Project, e.Note, e.AtMS, e.ID)
	if err != nil {
		return fmt.Errorf("store: put edit: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoEdit
	}
	return nil
}

// DeleteEdit removes one edit.
func (s *Store) DeleteEdit(id int64) error {
	res, err := s.db.Exec(`DELETE FROM edit WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete edit: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoEdit
	}
	return nil
}

// EditStamp changes whenever an edit is added, changed or removed.
func (s *Store) EditStamp() (n, sum int64, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(at_ms + id), 0) FROM edit`).Scan(&n, &sum)
	return n, sum, err
}

// Undo is one entry of the undo journal.
type Undo struct {
	ID   int64
	AtMS int64
	What string // shown to the user: "tag 3 spans Code/tock"
	Data string // JSON, the app's to read
}

// MaxUndo is how many entries the journal keeps.
const MaxUndo = 200

// PushUndo appends u and trims the journal to MaxUndo entries.
func (s *Store) PushUndo(u Undo) error {
	if _, err := s.db.Exec(`INSERT INTO undo(at_ms, what, data) VALUES(?, ?, ?)`, u.AtMS, u.What, u.Data); err != nil {
		return fmt.Errorf("store: push undo: %w", err)
	}
	_, err := s.db.Exec(`DELETE FROM undo WHERE id <= (SELECT MAX(id) FROM undo) - ?`, MaxUndo)
	return err
}

// Undos returns up to n entries, newest first.
func (s *Store) Undos(n int) ([]Undo, error) {
	rows, err := s.db.Query(`SELECT id, at_ms, what, data FROM undo ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Undo
	for rows.Next() {
		var u Undo
		if err := rows.Scan(&u.ID, &u.AtMS, &u.What, &u.Data); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DropUndo removes one entry.
func (s *Store) DropUndo(id int64) error {
	_, err := s.db.Exec(`DELETE FROM undo WHERE id = ?`, id)
	return err
}

// OverridesFor returns the manual labels of ids that have one.
func (s *Store) OverridesFor(ids []int64) ([]Override, error) {
	all, err := s.Overrides()
	if err != nil {
		return nil, err
	}
	var out []Override
	for _, id := range ids {
		if o, ok := all[id]; ok {
			out = append(out, o)
		}
	}
	return out, nil
}

// RestoreOverrides makes ids' manual labels exactly prev: ids without an
// entry in prev lose theirs.
func (s *Store) RestoreOverrides(ids []int64, prev []Override) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM override WHERE span_id = ?`, id); err != nil {
			return err
		}
	}
	for _, o := range prev {
		if _, err := tx.Exec(`INSERT INTO override(span_id, category, project, at) VALUES(?, NULLIF(?, ''), NULLIF(?, ''), ?)`,
			o.SpanID, o.Category, o.Project, o.At); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AwayByID returns one away row.
func (s *Store) AwayByID(id int64) (Away, error) {
	var a Away
	err := s.db.QueryRow(`SELECT id, start_ms, end_ms, category, project, at_ms FROM away WHERE id = ?`, id).
		Scan(&a.ID, &a.StartMS, &a.EndMS, &a.Category, &a.Project, &a.AtMS)
	if errors.Is(err, sql.ErrNoRows) {
		return Away{}, ErrNoAway
	}
	return a, err
}

// PutAway writes a back over its row exactly.
func (s *Store) PutAway(a Away) error {
	_, err := s.db.Exec(`UPDATE away SET category = ?, project = ?, at_ms = ? WHERE id = ?`, a.Category, a.Project, a.AtMS, a.ID)
	return err
}

// DeleteAway removes one away row.
func (s *Store) DeleteAway(id int64) error {
	_, err := s.db.Exec(`DELETE FROM away WHERE id = ?`, id)
	return err
}

// RestoreEdit puts a removed edit back under its own id, so it applies in
// its old place.
func (s *Store) RestoreEdit(e Edit) (int64, error) {
	_, err := s.db.Exec(`INSERT INTO edit(id, kind, start_ms, end_ms, category, project, note, at_ms) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Kind, e.StartMS, e.EndMS, e.Category, e.Project, e.Note, e.AtMS)
	if err != nil {
		return 0, fmt.Errorf("store: restore edit: %w", err)
	}
	return e.ID, nil
}
