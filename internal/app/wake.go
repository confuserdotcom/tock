package app

import (
	"errors"
	"fmt"
	"strconv"

	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/store"
)

// ErrNoWorkLabel means This is work has no label to file the view under.
var ErrNoWorkLabel = errors.New("no work label to file it under: label it in tock instead")

// SetPaper marks the running session (session, or 0 for whatever runs) as
// paper work: its idle time never counts as off task.
func (a *App) SetPaper(session int64) error {
	open, err := a.st.OpenSession()
	if err != nil {
		return err
	}
	if open == nil || (session > 0 && open.ID != session) {
		return store.ErrNoSession
	}
	return a.st.SetState(store.KeyFocusPaper, strconv.FormatInt(open.ID, 10), a.nowMS())
}

// StartSessionPaper starts a session of minutes, as paper work when paper
// is set.
func (a *App) StartSessionPaper(minutes int, paper bool) (focus.Session, error) {
	s, err := a.StartSession(minutes)
	if err != nil || !paper {
		return s, err
	}
	return s, a.SetPaper(s.ID)
}

// WakeIsWork files the time of key under want for good, as a learned rule
// (tock undo takes it back): the first safe rule Propose offers, else its
// first. It returns the rule.
func (a *App) WakeIsWork(key string, want classify.Want) (string, error) {
	if key == "" || want.Category == "" {
		return "", ErrNoWorkLabel
	}
	cands, err := a.Propose(key, want)
	if err != nil {
		return "", err
	}
	if len(cands) == 0 {
		return "", fmt.Errorf("app: this is work: no rule matches %q", key)
	}
	c := cands[0]
	for _, x := range cands {
		if x.Safe {
			c = x
			break
		}
	}
	if err := a.Learn(key, c, want); err != nil {
		return "", err
	}
	return c.Rule.Describe(), nil
}
