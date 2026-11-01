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
	// KeyFocusStartDay is the YYYY-MM-DD of the last start-of-day notice sent.
	KeyFocusStartDay = "focus_start_day"
	// KeyFocusMiddayDay is the YYYY-MM-DD of the last midday check-in sent.
	KeyFocusMiddayDay = "focus_midday_day"
	// KeyFocusOverDay is the YYYY-MM-DD of the last overworking notice sent.
	KeyFocusOverDay = "focus_over_day"
	// KeyFocusOverAt is the unix ms of the last overworking notice sent.
	KeyFocusOverAt = "focus_over_at"
	KeyFocusAwayAt = "focus_away_at"
	// KeyFocusBreakSnooze is when (ms) snoozed break reminders resume.
	KeyFocusBreakSnooze = "focus_break_snooze"
	// KeyFocusBreakTaken is when (ms) the user took a break from a break
	// reminder; tock owns it, trackd only reads it.
	KeyFocusBreakTaken = "focus_break_taken"
	// KeyFocusBreakUntil is when (ms) the break the user took ends; tock owns
	// it, trackd only reads it (so a click between trackd's read and its write
	// is never lost, as with KeyFocusDistractSnooze).
	KeyFocusBreakUntil = "focus_break_until"
	// KeyFocusDistractSnooze is when (ms) snoozed distraction notices of the
	// running session resume.
	KeyFocusDistractSnooze = "focus_distract_snooze"
	// KeyGuardSession is the id of the session whose start hooks ran.
	KeyGuardSession = "guard_session"
	// KeyGuardBlockAt is when (ms) the focus guard last blocked something.
	KeyGuardBlockAt = "guard_block_at"
)

var (
	// ErrSessionRunning means a manual session is already open.
	ErrSessionRunning = errors.New("session already running")
	// ErrNoSession means no manual session is open.
	ErrNoSession = errors.New("no session")
	// ErrSessionGone means no session has the id asked for.
	ErrSessionGone = errors.New("no such session")
	// ErrSessionOpen means the session is still running, so it cannot be rated
	// or noted yet.
	ErrSessionOpen = errors.New("session still running")
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
	Switches      int    // context switches in the session window; -1 = recorded before migration 0014
	Rating        int    // 1..5, 0 = unrated
	Note          string // the user's one line about the session
	RatedMS       int64  // when it was rated, 0 = unrated
	UrgesResisted int    // distraction notices answered Back to work
	UrgesGiven    int    // distraction notices answered Open anyway
}

const sessionCols = `id, kind, start_ms, planned_ms, end_ms, interruptions, alerted_ms, deep_ms, distract_ms, longest_ms, switches, rating, note, rated_ms, urges_resisted, urges_given`

func scanSession(sc interface{ Scan(...any) error }) (Session, error) {
	var x Session
	var planned, end, rating, ratedMS sql.NullInt64
	var note sql.NullString
	err := sc.Scan(&x.ID, &x.Kind, &x.StartMS, &planned, &end, &x.Interruptions, &x.AlertedMS,
		&x.DeepMS, &x.DistractMS, &x.LongestMS, &x.Switches, &rating, &note, &ratedMS,
		&x.UrgesResisted, &x.UrgesGiven)
	x.PlannedMS, x.EndMS = planned.Int64, end.Int64
	x.Rating, x.Note, x.RatedMS = int(rating.Int64), note.String, ratedMS.Int64
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
	// A new session starts with distraction notices on.
	if _, err := tx.Exec(`DELETE FROM daemon_state WHERE key=?`, KeyFocusDistractSnooze); err != nil {
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
	res, err := s.db.Exec(`UPDATE session SET end_ms=?, interruptions=?, alerted_ms=?, deep_ms=?, distract_ms=?, longest_ms=?, switches=?
		WHERE id=? AND end_ms IS NULL`,
		x.EndMS, x.Interruptions, x.AlertedMS, x.DeepMS, x.DistractMS, x.LongestMS, x.Switches, x.ID)
	return changedOne(res, err, "end session")
}

// AddAutoSession inserts a closed auto session. It reports false when an
// auto session with the same start exists.
func (s *Store) AddAutoSession(x Session) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO session(kind, start_ms, end_ms, deep_ms, distract_ms, longest_ms, switches)
		VALUES(?,?,?,?,?,?,?)`, SessionAuto, x.StartMS, x.EndMS, x.DeepMS, x.DistractMS, x.LongestMS, x.Switches)
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

// SessionByID returns the session with the id, or ErrSessionGone.
func (s *Store) SessionByID(id int64) (Session, error) {
	x, err := scanSession(s.db.QueryRow(`SELECT `+sessionCols+` FROM session WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionGone
	}
	if err != nil {
		return Session{}, fmt.Errorf("store: session by id: %w", err)
	}
	return x, nil
}

// LastFinishedSession returns the session that ended last, or nil when none
// has. A running session is never the last one.
func (s *Store) LastFinishedSession() (*Session, error) {
	x, err := scanSession(s.db.QueryRow(`SELECT ` + sessionCols + ` FROM session
		WHERE end_ms IS NOT NULL ORDER BY end_ms DESC, id DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: last finished session: %w", err)
	}
	return &x, nil
}

// RateSession stores a 1..5 rating on a finished session; the last rating
// wins. A nil note keeps the stored note, otherwise note replaces it. It only
// writes rating, rated_ms and note, never the counters trackd owns.
func (s *Store) RateSession(id int64, rating int, note *string, nowMS int64) error {
	var n any // a nil interface is SQL NULL, which COALESCE skips
	if note != nil {
		n = *note
	}
	res, err := s.db.Exec(`UPDATE session SET rating=?, rated_ms=?, note=COALESCE(?, note)
		WHERE id=? AND end_ms IS NOT NULL`, rating, nowMS, n, id)
	return s.finishedWrite(res, err, id, "rate session")
}

// SetNote stores the note of a finished session; "" clears it.
func (s *Store) SetNote(id int64, note string) error {
	res, err := s.db.Exec(`UPDATE session SET note=NULLIF(?, '') WHERE id=? AND end_ms IS NOT NULL`, note, id)
	return s.finishedWrite(res, err, id, "set session note")
}

// finishedWrite turns a guarded update that changed no row into
// ErrSessionGone or ErrSessionOpen.
func (s *Store) finishedWrite(res sql.Result, err error, id int64, what string) error {
	ok, err := changedOne(res, err, what)
	if err != nil || ok {
		return err
	}
	x, err := s.SessionByID(id)
	if err != nil {
		return err
	}
	if x.EndMS == 0 {
		return ErrSessionOpen
	}
	return fmt.Errorf("store: %s: no row changed", what)
}

// RatedSessions returns the finished, rated sessions in id order. The order
// is part of the weight fit's determinism contract.
func (s *Store) RatedSessions() ([]Session, error) {
	rows, err := s.db.Query(`SELECT ` + sessionCols + ` FROM session
		WHERE rating IS NOT NULL AND end_ms IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: rated sessions: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		x, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("store: rated sessions: %w", err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
