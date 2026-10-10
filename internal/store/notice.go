package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// daemon_state keys of the tock menu bar app.
const (
	// KeyBarBeat is the unix ms of the menu bar app's last heartbeat.
	KeyBarBeat = "bar_beat"
	// KeyBarNotify is its notification access: granted, denied or unknown.
	KeyBarNotify = "bar_notify"
)

// NoticeKeepMS is how long taken notices are kept.
const NoticeKeepMS = 7 * 24 * 3600_000

// ErrNoNotice means no notice has the id.
var ErrNoNotice = errors.New("no such notice")

// Notice is one notification queued for the menu bar app. Data is a small
// JSON object whose shape depends on Topic.
type Notice struct {
	ID          int64
	AtMS        int64
	Topic       string
	Title, Body string
	Data        string
	ShownMS     int64 // 0 until taken
}

// AddNotice queues n and returns its id.
func (s *Store) AddNotice(n Notice) (int64, error) {
	if n.Data == "" {
		n.Data = "{}"
	}
	r, err := s.db.Exec(`INSERT INTO notice(at_ms, topic, title, body, data) VALUES(?,?,?,?,?)`,
		n.AtMS, n.Topic, n.Title, n.Body, n.Data)
	if err != nil {
		return 0, fmt.Errorf("store: add notice: %w", err)
	}
	return r.LastInsertId()
}

// TakeNotices returns the untaken notices queued at or after since, oldest
// first, and marks every untaken notice taken at nowMS, so a notice that
// waited too long is dropped rather than shown late. Taken notices older
// than NoticeKeepMS are deleted.
func (s *Store) TakeNotices(nowMS, since int64) ([]Notice, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, at_ms, topic, title, body, data FROM notice
		WHERE shown_ms IS NULL AND at_ms >= ? ORDER BY id`, since)
	if err != nil {
		return nil, err
	}
	var out []Notice
	for rows.Next() {
		n := Notice{ShownMS: nowMS}
		if err := rows.Scan(&n.ID, &n.AtMS, &n.Topic, &n.Title, &n.Body, &n.Data); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE notice SET shown_ms = ? WHERE shown_ms IS NULL`, nowMS); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM notice WHERE shown_ms IS NOT NULL AND at_ms < ?`, nowMS-NoticeKeepMS); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: take notices: %w", err)
	}
	return out, nil
}

// Notice returns the notice with id, or ErrNoNotice.
func (s *Store) Notice(id int64) (Notice, error) {
	var n Notice
	var shown sql.NullInt64
	err := s.db.QueryRow(`SELECT id, at_ms, topic, title, body, data, shown_ms FROM notice WHERE id = ?`, id).
		Scan(&n.ID, &n.AtMS, &n.Topic, &n.Title, &n.Body, &n.Data, &shown)
	if errors.Is(err, sql.ErrNoRows) {
		return Notice{}, ErrNoNotice
	}
	if err != nil {
		return Notice{}, fmt.Errorf("store: notice: %w", err)
	}
	n.ShownMS = shown.Int64
	return n, nil
}
