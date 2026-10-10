package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Session kinds.
const (
	SessionManual = "manual"
	SessionAuto   = "auto"
)

// daemon_state keys of the focus engine.
const (
	// KeyFocusBreakFrom is the start (ms) of the activity run the last break
	// reminder belongs to.
	KeyFocusBreakFrom = "focus_break_from"
	// KeyFocusBreakAt is when (ms) the last break reminder or session end fired.
	KeyFocusBreakAt = "focus_break_at"
	// KeyFocusRecapDay is the YYYY-MM-DD of the last recap sent.
	KeyFocusRecapDay = "focus_recap_day"
	KeyFocusAwayAt   = "focus_away_at"
)

var (
	// ErrSessionRunning means a manual session is already open.
	ErrSessionRunning = errors.New("session already running")
	// ErrNoSession means no manual session is open.
	ErrNoSession = errors.New("no session")
)

// Session is one focus session. focus.Session has the same fields in the
// same order so the two convert directly; keep them in step.
type Session struct {
	ID            int64
	Kind          string
	StartMS       int64
	PlannedMS     int64 // 0 for auto sessions
	EndMS         int64 // 0 while running
	Interruptions int
	AlertedMS     int64
	DeepMS        int64
	DistractMS    int64
	LongestMS     int64
}

const sessionCols = `id, kind, start_ms, planned_ms, end_ms, interruptions, alerted_ms, deep_ms, distract_ms, longest_ms`

func scanSession(sc interface{ Scan(...any) error }) (Session, error) {
	var x Session
	var planned, end sql.NullInt64
	err := sc.Scan(&x.ID, &x.Kind, &x.StartMS, &planned, &end, &x.Interruptions, &x.AlertedMS,
		&x.DeepMS, &x.DistractMS, &x.LongestMS)
	x.PlannedMS, x.EndMS = planned.Int64, end.Int64
	return x, err
}

// StartSession inserts a running manual session. It fails with
// ErrSessionRunning when one is open.
func (s *Store) StartSession(startMS, plannedMS int64) (Session, error) {
	tx, err := s.db.Begin() // _txlock=immediate: the check and insert are atomic
	if err != nil {
		return Session{}, fmt.Errorf("store: start session: %w", err)
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM session WHERE end_ms IS NULL`).Scan(&n); err != nil {
		return Session{}, fmt.Errorf("store: start session: %w", err)
	}
	if n > 0 {
		return Session{}, ErrSessionRunning
	}
	res, err := tx.Exec(`INSERT INTO session(kind, start_ms, planned_ms) VALUES(?,?,?)`,
		SessionManual, startMS, plannedMS)
	if err != nil {
		return Session{}, fmt.Errorf("store: start session: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Session{}, fmt.Errorf("store: start session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Session{}, fmt.Errorf("store: start session: %w", err)
	}
	return Session{ID: id, Kind: SessionManual, StartMS: startMS, PlannedMS: plannedMS}, nil
}

// OpenSession returns the running manual session, or nil.
func (s *Store) OpenSession() (*Session, error) {
	x, err := scanSession(s.db.QueryRow(`SELECT ` + sessionCols + ` FROM session WHERE end_ms IS NULL`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: open session: %w", err)
	}
	return &x, nil
}

// UpdateSession writes the counters of a running session. It reports false
// when the session is no longer running.
func (s *Store) UpdateSession(x Session) (bool, error) {
	res, err := s.db.Exec(`UPDATE session SET interruptions=?, alerted_ms=?, deep_ms=?, distract_ms=?, longest_ms=?
		WHERE id=? AND end_ms IS NULL`,
		x.Interruptions, x.AlertedMS, x.DeepMS, x.DistractMS, x.LongestMS, x.ID)
	return changedOne(res, err, "update session")
}

// EndSession closes a running session at x.EndMS with x's counters. It
// reports false when the session was already closed.
func (s *Store) EndSession(x Session) (bool, error) {
	res, err := s.db.Exec(`UPDATE session SET end_ms=?, interruptions=?, alerted_ms=?, deep_ms=?, distract_ms=?, longest_ms=?
		WHERE id=? AND end_ms IS NULL`,
		x.EndMS, x.Interruptions, x.AlertedMS, x.DeepMS, x.DistractMS, x.LongestMS, x.ID)
	return changedOne(res, err, "end session")
}

// AddAutoSession inserts a closed auto session. It reports false when an
// auto session with the same start exists.
func (s *Store) AddAutoSession(x Session) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO session(kind, start_ms, end_ms, deep_ms, distract_ms, longest_ms)
		VALUES(?,?,?,?,?,?)`, SessionAuto, x.StartMS, x.EndMS, x.DeepMS, x.DistractMS, x.LongestMS)
	return changedOne(res, err, "add auto session")
}

func changedOne(res sql.Result, err error, what string) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("store: %s: %w", what, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: %s: %w", what, err)
	}
	return n == 1, nil
}

// Sessions returns the sessions overlapping [from, to), by start. A running
// session overlaps every window after its start.
func (s *Store) Sessions(from, to int64) ([]Session, error) {
	rows, err := s.db.Query(`SELECT `+sessionCols+` FROM session
		WHERE start_ms < ? AND (end_ms IS NULL OR end_ms > ?) ORDER BY start_ms, id`, to, from)
	if err != nil {
		return nil, fmt.Errorf("store: sessions: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		x, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("store: sessions: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
