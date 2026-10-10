package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

// Event is one calendar event.
type Event struct {
	ID             int64
	StartMS, EndMS int64
	Title          string
	Calendar       string
}

func eventLess(a, b Event) bool {
	if a.StartMS != b.StartMS {
		return a.StartMS < b.StartMS
	}
	if a.EndMS != b.EndMS {
		return a.EndMS < b.EndMS
	}
	if a.Title != b.Title {
		return a.Title < b.Title
	}
	return a.Calendar < b.Calendar
}

// SyncEvents makes the rows that start in [from, to) equal to evs (whose
// IDs are ignored; events that start outside the window are dropped). When
// they already are, it writes nothing and reports changed false. Rows
// outside the window stay.
func (s *Store) SyncEvents(from, to int64, evs []Event) (changed bool, err error) {
	want := make([]Event, 0, len(evs))
	for _, e := range evs {
		if e.StartMS >= from && e.StartMS < to && e.EndMS > e.StartMS {
			e.ID = 0
			want = append(want, e)
		}
	}
	sort.Slice(want, func(i, j int) bool { return eventLess(want[i], want[j]) })

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT start_ms, end_ms, title, calendar FROM event
		WHERE start_ms >= ? AND start_ms < ? ORDER BY start_ms, end_ms, title, calendar`, from, to)
	if err != nil {
		return false, err
	}
	var have []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.StartMS, &e.EndMS, &e.Title, &e.Calendar); err != nil {
			rows.Close()
			return false, err
		}
		have = append(have, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(have) == len(want) {
		same := true
		for i := range have {
			same = same && have[i] == want[i]
		}
		if same {
			return false, nil
		}
	}
	if _, err := tx.Exec(`DELETE FROM event WHERE start_ms >= ? AND start_ms < ?`, from, to); err != nil {
		return false, fmt.Errorf("store: sync events: %w", err)
	}
	for _, e := range want {
		if _, err := tx.Exec(`INSERT INTO event(start_ms, end_ms, title, calendar) VALUES(?, ?, ?, ?)`,
			e.StartMS, e.EndMS, e.Title, e.Calendar); err != nil {
			return false, fmt.Errorf("store: sync events: %w", err)
		}
	}
	return true, tx.Commit()
}

// ErrNoEvent is returned when an event row does not exist.
var ErrNoEvent = errors.New("store: no such calendar event")

// Event returns row id.
func (s *Store) Event(id int64) (Event, error) {
	var e Event
	err := s.db.QueryRow(`SELECT id, start_ms, end_ms, title, calendar FROM event WHERE id = ?`, id).
		Scan(&e.ID, &e.StartMS, &e.EndMS, &e.Title, &e.Calendar)
	if errors.Is(err, sql.ErrNoRows) {
		return Event{}, ErrNoEvent
	}
	return e, err
}

// Events returns the rows overlapping [from, to), in start order.
func (s *Store) Events(from, to int64) ([]Event, error) {
	rows, err := s.db.Query(`SELECT id, start_ms, end_ms, title, calendar FROM event
		WHERE end_ms > ? AND start_ms < ? ORDER BY start_ms, end_ms, id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.StartMS, &e.EndMS, &e.Title, &e.Calendar); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EventStamp changes whenever SyncEvents changes the table: the row count
// and the largest id.
func (s *Store) EventStamp() (n, maxID int64, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(id), 0) FROM event`).Scan(&n, &maxID)
	return n, maxID, err
}
