package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// RecommendKeepMS is how long recommend rows are kept.
const RecommendKeepMS = 60 * 24 * 3600_000

// ErrRecommendGone: no recommendation row has this id.
var ErrRecommendGone = errors.New("store: no such recommendation")

// Recommend is one recommendation notice trackd sent. Kind is "focus" or
// "break"; Weekday (0 = Sunday) and Hour are local at sending; Reply is ""
// until answered, then "accept" or "dismiss", set once. recommend.Rec has the
// same fields in the same order; keep them in step.
type Recommend struct {
	ID       int64
	Kind     string
	AtMS     int64
	Weekday  int
	Hour     int
	LengthMS int64
	Reply    string
	ReplyMS  int64
}

const recommendCols = `id, kind, at_ms, weekday, hour, length_ms, reply, reply_ms`

// AddRecommend inserts an unanswered row and returns its id. Rows older than
// RecommendKeepMS before r.AtMS are deleted in the same transaction.
func (s *Store) AddRecommend(r Recommend) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("store: add recommend: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM recommend WHERE at_ms < ?`, r.AtMS-RecommendKeepMS); err != nil {
		return 0, fmt.Errorf("store: add recommend: %w", err)
	}
	res, err := tx.Exec(`INSERT INTO recommend(kind, at_ms, weekday, hour, length_ms, reply, reply_ms) VALUES(?, ?, ?, ?, ?, '', 0)`,
		r.Kind, r.AtMS, r.Weekday, r.Hour, r.LengthMS)
	if err != nil {
		return 0, fmt.Errorf("store: add recommend: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: add recommend: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: add recommend: %w", err)
	}
	return id, nil
}

func scanRecommend(sc interface{ Scan(...any) error }) (Recommend, error) {
	var r Recommend
	err := sc.Scan(&r.ID, &r.Kind, &r.AtMS, &r.Weekday, &r.Hour, &r.LengthMS, &r.Reply, &r.ReplyMS)
	return r, err
}

// Recommend returns the row with this id, ErrRecommendGone when there is none.
func (s *Store) Recommend(id int64) (Recommend, error) {
	r, err := scanRecommend(s.db.QueryRow(`SELECT `+recommendCols+` FROM recommend WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Recommend{}, ErrRecommendGone
	}
	if err != nil {
		return Recommend{}, fmt.Errorf("store: recommend: %w", err)
	}
	return r, nil
}

// Recommends returns the rows sent at or after fromMS, in (at_ms, id) order.
func (s *Store) Recommends(fromMS int64) ([]Recommend, error) {
	rows, err := s.db.Query(`SELECT `+recommendCols+` FROM recommend WHERE at_ms >= ? ORDER BY at_ms, id`, fromMS)
	if err != nil {
		return nil, fmt.Errorf("store: recommends: %w", err)
	}
	defer rows.Close()
	var out []Recommend
	for rows.Next() {
		r, err := scanRecommend(rows)
		if err != nil {
			return nil, fmt.Errorf("store: recommends: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: recommends: %w", err)
	}
	return out, nil
}

// ReplyRecommend sets the reply of row id to reply at nowMS and reports
// whether it did: only while the row is unanswered and was sent after sinceMS,
// so a reply lands once even when notification, menu, TUI and CLI race.
func (s *Store) ReplyRecommend(id int64, reply string, nowMS, sinceMS int64) (bool, error) {
	res, err := s.db.Exec(`UPDATE recommend SET reply = ?, reply_ms = ? WHERE id = ? AND reply = '' AND at_ms > ?`,
		reply, nowMS, id, sinceMS)
	if err != nil {
		return false, fmt.Errorf("store: reply recommend: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: reply recommend: %w", err)
	}
	return n == 1, nil
}

// UnreplyRecommend puts reply ” and reply_ms 0 back, only while the reply is
// still the given one: for an accept whose action could not be carried out.
func (s *Store) UnreplyRecommend(id int64, reply string) error {
	if _, err := s.db.Exec(`UPDATE recommend SET reply = '', reply_ms = 0 WHERE id = ? AND reply = ?`, id, reply); err != nil {
		return fmt.Errorf("store: unreply recommend: %w", err)
	}
	return nil
}

// DeleteRecommend deletes row id, only while it is unanswered: for a
// recommendation that was written but could not be handed over, so the next
// tick suggests again. A missing or answered row is not an error.
func (s *Store) DeleteRecommend(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM recommend WHERE id = ? AND reply = ''`, id); err != nil {
		return fmt.Errorf("store: delete recommend: %w", err)
	}
	return nil
}
