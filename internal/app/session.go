package app

import (
	"errors"

	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

// SessionByID returns the session with the id, or store.ErrSessionGone.
func (a *App) SessionByID(id int64) (focus.Session, error) {
	s, err := a.st.SessionByID(id)
	return focus.Session(s), err
}

// RunningSessionID is the id of the running manual session, 0 when none runs.
func (a *App) RunningSessionID() (int64, error) {
	open, err := a.st.OpenSession()
	if err != nil || open == nil {
		return 0, err
	}
	return open.ID, nil
}

// LastSession returns the session that ended last, or nil when none has. A
// running session is never the last one.
func (a *App) LastSession() (*focus.Session, error) {
	s, err := a.st.LastFinishedSession()
	if err != nil || s == nil {
		return nil, err
	}
	f := focus.Session(*s)
	return &f, nil
}

// RateSession rates a finished session 1..5 (the last rating wins) and
// returns it with the rating it replaced (0 when unrated). A non-nil note is
// cleaned to one safe line of at most report.MaxNoteRunes runes and replaces
// the stored one; nil keeps it. A running session fails with
// store.ErrSessionOpen, an unknown id with store.ErrSessionGone.
func (a *App) RateSession(id int64, rating int, note *string) (s focus.Session, prev int, err error) {
	if rating < 1 || rating > 5 {
		return focus.Session{}, 0, errors.New("rating must be 1 to 5")
	}
	old, err := a.st.SessionByID(id)
	if err != nil {
		return focus.Session{}, 0, err
	}
	if old.EndMS == 0 {
		return focus.Session{}, 0, store.ErrSessionOpen
	}
	if note != nil {
		c, _ := report.CleanNote(*note)
		note = &c
	}
	if err := a.st.RateSession(id, rating, note, a.nowMS()); err != nil {
		return focus.Session{}, 0, err
	}
	s, err = a.SessionByID(id)
	return s, old.Rating, err
}

// SetNote replaces the note of a finished session with the cleaned text; an
// empty text clears it.
func (a *App) SetNote(id int64, note string) (focus.Session, error) {
	c, _ := report.CleanNote(note)
	if err := a.st.SetNote(id, c); err != nil {
		return focus.Session{}, err
	}
	return a.SessionByID(id)
}

// Weights returns the quality weights in force: the defaults until
// focus.MinRated eligible rated sessions exist, then the fit to the ratings.
// rated is how many sessions were eligible and fitted tells which case it is.
func (a *App) Weights() (w focus.Weights, rated int, fitted bool, err error) {
	ss, err := a.st.RatedSessions()
	if err != nil {
		return focus.Weights{}, 0, false, err
	}
	w, rated = focus.FitWeights(focusSessions(ss))
	return w, rated, rated >= focus.MinRated, nil
}

// SessionDetail is the card model of a finished session: its quality with the
// current weights and the breakdown of its window. It reads one small window,
// so callers ask for the one session on screen, never the whole week. A
// running session fails with store.ErrSessionOpen, an unknown id with
// store.ErrSessionGone.
func (a *App) SessionDetail(id int64) (report.SessionDetail, error) {
	s, err := a.SessionByID(id)
	if err != nil {
		return report.SessionDetail{}, err
	}
	if s.EndMS == 0 {
		return report.SessionDetail{}, store.ErrSessionOpen
	}
	vs, labels, err := a.labelled(s.StartMS, s.EndMS, a.nowMS())
	if err != nil {
		return report.SessionDetail{}, err
	}
	top := focus.Window(vs, labels, s.StartMS, s.EndMS, a.cfg.Focus)
	w, _, _, err := a.Weights()
	if err != nil {
		return report.SessionDetail{}, err
	}
	return report.SessionDetail{Sess: s, Q: focus.Quality(s, w), Top: top}, nil
}

// RunningSession is the running manual session, nil when none runs.
func (a *App) RunningSession() (*focus.Session, error) {
	open, err := a.st.OpenSession()
	if err != nil || open == nil {
		return nil, err
	}
	s := focus.Session(*open)
	return &s, nil
}
