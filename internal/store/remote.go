package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// RemoteView is one view of another machine's activity.
type RemoteView struct {
	ID                int64
	Machine, Day      string
	StartMS, EndMS    int64
	Bundle, App       string
	Title, URL        *string
	Category, Project string
}

// RemoteFile is what was read of one machine's day file.
type RemoteFile struct {
	MtimeMS, Size int64
}

// RemoteFileSeen returns what was last read of machine's day file.
func (s *Store) RemoteFileSeen(machine, day string) (RemoteFile, bool, error) {
	var f RemoteFile
	err := s.db.QueryRow(`SELECT mtime_ms, size FROM remote_file WHERE machine = ? AND day = ?`, machine, day).
		Scan(&f.MtimeMS, &f.Size)
	if errors.Is(err, sql.ErrNoRows) {
		return f, false, nil
	}
	if err != nil {
		return f, false, fmt.Errorf("store: remote file: %w", err)
	}
	return f, true, nil
}

// ReplaceRemoteDay makes machine's views of day equal to vs (their IDs,
// Machine and Day are ignored) and records the file read.
func (s *Store) ReplaceRemoteDay(machine, day string, f RemoteFile, vs []RemoteView) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM remote_view WHERE machine = ? AND day = ?`, machine, day); err != nil {
		return err
	}
	ins, err := tx.Prepare(`INSERT INTO remote_view(machine, day, start_ms, end_ms, bundle, app, title, url, category, project)
		VALUES(?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	for _, v := range vs {
		if v.EndMS <= v.StartMS {
			continue
		}
		if _, err := ins.Exec(machine, day, v.StartMS, v.EndMS, v.Bundle, v.App, v.Title, v.URL, v.Category, v.Project); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO remote_file(machine, day, mtime_ms, size) VALUES(?,?,?,?)
		ON CONFLICT(machine, day) DO UPDATE SET mtime_ms = excluded.mtime_ms, size = excluded.size`,
		machine, day, f.MtimeMS, f.Size); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: replace remote day: %w", err)
	}
	return nil
}

// RemoteViews returns the remote views overlapping [from, to), in start
// order.
func (s *Store) RemoteViews(from, to int64) ([]RemoteView, error) {
	rows, err := s.db.Query(`SELECT id, machine, day, start_ms, end_ms, bundle, app, title, url, category, project
		FROM remote_view WHERE start_ms < ? AND end_ms > ? ORDER BY start_ms, id`, to, from)
	if err != nil {
		return nil, fmt.Errorf("store: remote views: %w", err)
	}
	defer rows.Close()
	var out []RemoteView
	for rows.Next() {
		var v RemoteView
		var title, url sql.NullString
		if err := rows.Scan(&v.ID, &v.Machine, &v.Day, &v.StartMS, &v.EndMS, &v.Bundle, &v.App, &title, &url,
			&v.Category, &v.Project); err != nil {
			return nil, err
		}
		if title.Valid {
			v.Title = &title.String
		}
		if url.Valid {
			v.URL = &url.String
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RemoteStamp changes whenever the remote views change: their count and
// largest id.
func (s *Store) RemoteStamp() (n, maxID int64, err error) {
	err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(id), 0) FROM remote_view`).Scan(&n, &maxID)
	return n, maxID, err
}

// RemoteMachine is one other machine and its latest activity.
type RemoteMachine struct {
	Name   string
	LastMS int64 // end of its latest view
	Days   int   // day files read
}

// RemoteMachines lists the other machines read so far, by name.
func (s *Store) RemoteMachines() ([]RemoteMachine, error) {
	rows, err := s.db.Query(`SELECT f.machine, COUNT(*),
		COALESCE((SELECT MAX(end_ms) FROM remote_view v WHERE v.machine = f.machine), 0)
		FROM remote_file f GROUP BY f.machine ORDER BY f.machine`)
	if err != nil {
		return nil, fmt.Errorf("store: remote machines: %w", err)
	}
	defer rows.Close()
	var out []RemoteMachine
	for rows.Next() {
		var m RemoteMachine
		if err := rows.Scan(&m.Name, &m.Days, &m.LastMS); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
