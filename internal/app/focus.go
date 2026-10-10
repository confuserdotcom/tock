package app

import (
	"fmt"
	"strconv"
	"time"

	"tock/internal/config"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/store"
)

// FocusTick runs the focus engine once, for trackd: it records the call
// going on (call is true while a camera or microphone is in use), reloads
// changed rule and config files, applies the engine's store events and
// returns the notifications to send. It stops at the first failed write,
// before any notification. A session notification is dropped when tock closed that
// session between the read and the write.
func (a *App) FocusTick(call bool) ([]focus.Event, error) {
	notes, _, err := a.GuardTick(call)
	return notes, err
}

// GuardTick is FocusTick that also returns what the focus guard asks of
// trackd (session hooks, a block). The guard's state is saved before it
// returns.
func (a *App) GuardTick(call bool) ([]focus.Event, focus.Guard, error) {
	a.Reload() // a failure is kept in RulesErr / ConfigErr; the last good files stay
	nowT := a.now()
	now := nowT.UnixMilli()
	day := a.midnight(nowT)
	if call {
		if err := a.st.RecordCall(now); err != nil {
			return nil, focus.Guard{}, err
		}
	}
	open, err := a.st.OpenSession()
	if err != nil {
		return nil, focus.Guard{}, err
	}
	from := day.UnixMilli()
	if open != nil && open.StartMS < from {
		from = open.StartMS
	}
	vs, labels, err := a.labelled(from, now, now)
	if err != nil {
		return nil, focus.Guard{}, err
	}
	ss, err := a.st.Sessions(from, now+1)
	if err != nil {
		return nil, focus.Guard{}, err
	}
	st, err := a.focusState()
	if err != nil {
		return nil, focus.Guard{}, err
	}
	in := focus.Input{Now: now, Day: day, Views: vs, Labels: labels, Sessions: focusSessions(ss),
		Call: call, State: st, Cfg: a.cfg.Focus}
	abs, err := a.pending(vs, from, now)
	if err != nil {
		return nil, focus.Guard{}, err
	}
	if n := len(abs); n > 0 {
		in.Absence = &abs[n-1]
	}
	if focus.RecapDue(in) {
		d, err := a.Day(nowT)
		if err != nil {
			return nil, focus.Guard{}, err
		}
		in.RecapBody = report.Recap(d)
	}

	gs, err := a.guardState()
	if err != nil {
		return nil, focus.Guard{}, err
	}
	guard := focus.GuardStep(in, gs)

	var notes []focus.Event
	stale := false
	for _, ev := range focus.Step(in) {
		var ok bool
		var err error
		switch ev.Kind {
		case focus.SessionUpdate:
			ok, err = a.st.UpdateSession(store.Session(ev.Sess))
			stale = stale || (err == nil && !ok)
		case focus.SessionEnd:
			ok, err = a.st.EndSession(store.Session(ev.Sess))
			stale = stale || (err == nil && !ok)
		case focus.AutoSession:
			_, err = a.st.AddAutoSession(store.Session(ev.Sess))
		case focus.StateUpdate:
			err = a.setFocusState(ev.State, now)
		case focus.Notify:
			if !(stale && ev.OfSession) {
				notes = append(notes, ev)
			}
		}
		if err != nil {
			return nil, focus.Guard{}, err
		}
	}
	if guard.Changed {
		if err := a.setGuardState(guard.State, now); err != nil {
			return nil, focus.Guard{}, err
		}
	}
	return notes, guard, nil
}

func focusSessions(ss []store.Session) []focus.Session {
	out := make([]focus.Session, len(ss))
	for i, s := range ss {
		out[i] = focus.Session(s)
	}
	return out
}

func (a *App) focusState() (focus.State, error) {
	m, err := a.st.States()
	if err != nil {
		return focus.State{}, err
	}
	from, _ := strconv.ParseInt(m[store.KeyFocusBreakFrom], 10, 64)
	at, _ := strconv.ParseInt(m[store.KeyFocusBreakAt], 10, 64)
	away, _ := strconv.ParseInt(m[store.KeyFocusAwayAt], 10, 64)
	snooze, _ := strconv.ParseInt(m[store.KeyFocusBreakSnooze], 10, 64)
	return focus.State{BreakFrom: from, BreakAt: at, RecapDay: m[store.KeyFocusRecapDay], AwayAt: away,
		SnoozeUntil: snooze}, nil
}

func (a *App) setFocusState(s focus.State, nowMS int64) error {
	for _, kv := range [][2]string{
		{store.KeyFocusBreakFrom, strconv.FormatInt(s.BreakFrom, 10)},
		{store.KeyFocusBreakAt, strconv.FormatInt(s.BreakAt, 10)},
		{store.KeyFocusRecapDay, s.RecapDay},
		{store.KeyFocusAwayAt, strconv.FormatInt(s.AwayAt, 10)},
		{store.KeyFocusBreakSnooze, strconv.FormatInt(s.SnoozeUntil, 10)},
	} {
		if err := a.st.SetState(kv[0], kv[1], nowMS); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) guardState() (focus.GuardState, error) {
	m, err := a.st.States()
	if err != nil {
		return focus.GuardState{}, err
	}
	id, _ := strconv.ParseInt(m[store.KeyGuardSession], 10, 64)
	at, _ := strconv.ParseInt(m[store.KeyGuardBlockAt], 10, 64)
	return focus.GuardState{HookSession: id, BlockAt: at}, nil
}

func (a *App) setGuardState(s focus.GuardState, nowMS int64) error {
	if err := a.st.SetState(store.KeyGuardSession, strconv.FormatInt(s.HookSession, 10), nowMS); err != nil {
		return err
	}
	return a.st.SetState(store.KeyGuardBlockAt, strconv.FormatInt(s.BlockAt, 10), nowMS)
}

// SnoozeBreak holds break reminders back for d from now.
func (a *App) SnoozeBreak(d time.Duration) error {
	now := a.nowMS()
	return a.st.SetState(store.KeyFocusBreakSnooze, strconv.FormatInt(now+d.Milliseconds(), 10), now)
}

// StartSession starts a manual session of minutes minutes now. It fails
// with store.ErrSessionRunning when one is running.
func (a *App) StartSession(minutes int) (focus.Session, error) {
	if minutes < 1 || minutes > config.MaxSessionMin {
		return focus.Session{}, fmt.Errorf("session length %d min, want 1 to %d", minutes, config.MaxSessionMin)
	}
	now := a.nowMS()
	s, err := a.st.StartSession(now, now+int64(minutes)*60_000)
	return focus.Session(s), err
}

// StopSession ends the running session now (at its planned end if that has
// passed, as when trackd was down) with its final counters. It fails with
// store.ErrNoSession when none is running.
func (a *App) StopSession() (focus.Session, error) {
	open, err := a.st.OpenSession()
	if err != nil {
		return focus.Session{}, err
	}
	if open == nil {
		return focus.Session{}, store.ErrNoSession
	}
	now := a.nowMS()
	s, err := a.live(focus.Session(*open), now)
	if err != nil {
		return focus.Session{}, err
	}
	s.EndMS = max(min(now, s.PlannedMS), s.StartMS)
	ok, err := a.st.EndSession(store.Session(s))
	if err != nil {
		return focus.Session{}, err
	}
	if !ok {
		return focus.Session{}, store.ErrNoSession
	}
	return s, nil
}

// live fills a running session's deep, distraction and longest times up to
// nowMS (capped at its planned end).
func (a *App) live(s focus.Session, nowMS int64) (focus.Session, error) {
	end := max(min(nowMS, s.PlannedMS), s.StartMS)
	vs, labels, err := a.labelled(s.StartMS, end, nowMS)
	if err != nil {
		return s, err
	}
	ws := focus.WindowStats(vs, labels, s.StartMS, end, a.cfg.Focus)
	s.DeepMS, s.DistractMS, s.LongestMS = ws.DeepMS, ws.DistractMS, ws.LongestMS
	return s, nil
}

// Focus reports the running session (live counters) and this Monday-start
// week's closed sessions.
func (a *App) Focus(now time.Time) (report.Focus, error) {
	nowMS := now.UnixMilli()
	today := a.midnight(now)
	ss, err := a.st.Sessions(report.MondayOf(today).UnixMilli(), nowMS+1)
	if err != nil {
		return report.Focus{}, err
	}
	f := report.Focus{NowMS: nowMS}
	for i := len(ss) - 1; i >= 0; i-- {
		s := focus.Session(ss[i])
		switch {
		case s.EndMS == 0:
			live, err := a.live(s, nowMS)
			if err != nil {
				return report.Focus{}, err
			}
			f.Open = &live
		case s.StartMS >= today.UnixMilli():
			f.Today = append(f.Today, s)
		default:
			f.Week = append(f.Week, s)
		}
	}
	return f, nil
}

// Recap is the recap text of the calendar day of day.
func (a *App) Recap(day time.Time) (string, error) {
	d, err := a.Day(day)
	if err != nil {
		return "", err
	}
	return report.Recap(d), nil
}
