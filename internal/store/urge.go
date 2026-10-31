package store

import "fmt"

// AddUrge counts one answer to an urge-surfing notice on session id, running
// or finished: resisted ("Back to work") or, when given is true, given in
// ("Open anyway"). It returns ErrSessionGone when no session has the id.
func (s *Store) AddUrge(id int64, given bool) error {
	col := "urges_resisted" // one of two literals, never input
	if given {
		col = "urges_given"
	}
	res, err := s.db.Exec(`UPDATE session SET `+col+` = `+col+` + 1 WHERE id=?`, id)
	ok, err := changedOne(res, err, "add urge")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("store: add urge: %w", ErrSessionGone)
	}
	return nil
}
