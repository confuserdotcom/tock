package app

import (
	"strconv"
	"time"

	"tock/internal/store"
)

// SnoozeDistract holds the distraction notices of the running session back
// for d from now and returns when they resume, in the App clock's zone.
// session is the id of the session a notice was about; 0 means whatever
// runs. It fails with store.ErrNoSession when no session runs or the running
// one is not session, so a late click on an old notice snoozes nothing.
func (a *App) SnoozeDistract(session int64, d time.Duration) (time.Time, error) {
	open, err := a.st.OpenSession()
	if err != nil {
		return time.Time{}, err
	}
	if open == nil || (session > 0 && open.ID != session) {
		return time.Time{}, store.ErrNoSession
	}
	until := a.now().Add(d)
	if err := a.st.SetState(store.KeyFocusDistractSnooze, strconv.FormatInt(until.UnixMilli(), 10), a.nowMS()); err != nil {
		return time.Time{}, err
	}
	return until, nil
}

// ResumeDistract ends a distraction snooze now. It reports whether
// notices were snoozed.
func (a *App) ResumeDistract() (bool, error) {
	until, err := a.distractSnooze()
	if err != nil {
		return false, err
	}
	now := a.nowMS()
	if err := a.st.SetState(store.KeyFocusDistractSnooze, "0", now); err != nil {
		return false, err
	}
	return until > now, nil
}

// distractSnooze is when the distraction snooze ends (ms), 0 when absent.
func (a *App) distractSnooze() (int64, error) {
	m, err := a.st.States()
	if err != nil {
		return 0, err
	}
	until, _ := strconv.ParseInt(m[store.KeyFocusDistractSnooze], 10, 64)
	return until, nil
}
