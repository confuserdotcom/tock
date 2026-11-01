package app

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/stats"
	"tock/internal/store"
)

// FocusTick runs the focus engine once, for trackd: it records the call
// going on (call is true while a camera or microphone is in use), reloads
// changed rule and config files, applies the engine's store events and
// returns the notifications to send. It stops at the first failed write,
// before any notification; the one exception is a slot notice: a failure to
// mark or start one slot is kept in RoutineErr and the other slots go on. A
// session notification is dropped when tock closed that session between the
// read and the write.
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
		Call: call, State: st, Cfg: a.cfg.Focus, Routine: a.cfg.Routine, Hours: a.cfg.Hours}
	if open != nil && now >= open.PlannedMS {
		// The open session ends on this tick: its done notice is scored with
		// the current weights. On every other tick no event uses them, so the
		// rated sessions are not read (trackd's idle budget).
		w, _, _, err := a.Weights()
		if err != nil {
			return nil, focus.Guard{}, err
		}
		in.Weights = w
	}
	abs, err := a.pending(vs, from, now)
	if err != nil {
		return nil, focus.Guard{}, err
	}
	if n := len(abs); n > 0 {
		in.Absence = &abs[n-1]
		in.AbsenceBefore = classify.Before(vs, labels, abs[n-1].From)
	}
	// Today's report is built at most once per tick, and only when a notice
	// that needs it is due.
	var today *report.Day
	todayDay := func() (report.Day, error) {
		if today == nil {
			d, err := a.Day(nowT)
			if err != nil {
				return report.Day{}, err
			}
			today = &d
		}
		return *today, nil
	}
	if !a.cfg.Hours.Off(day) && focus.RecapDue(in) {
		d, err := todayDay()
		if err != nil {
			return nil, focus.Guard{}, err
		}
		in.RecapBody = report.Recap(d)
	}
	a.routineBodies(&in, nowT, todayDay)
	a.slotInput(&in, nowT)
	a.recInput(&in, nowT)

	gs, err := a.guardState()
	if err != nil {
		return nil, focus.Guard{}, err
	}
	guard := focus.GuardStep(in, gs)

	var notes, slotEvs, recEvs []focus.Event
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
		case focus.SlotDue:
			slotEvs = append(slotEvs, ev) // marked and started below, once the tick's writes are done
		case focus.RecDue:
			recEvs = append(recEvs, ev) // its row is written below, after the slot notices
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
	// Slot notices come last: a notice is marked as sent only when nothing
	// that follows can fail the tick and drop it. One slot's failure is kept
	// as the tick's RoutineErr and leaves that slot for the next tick of its
	// window; the other slots' notices go out.
	for _, ev := range slotEvs {
		if n, ok := a.sendSlot(ev, now); ok {
			notes = append(notes, n)
		}
	}
	// Recommendation notices follow: the row is written only now, so nothing
	// that failed above leaves a row for a notice that was never queued.
	for _, ev := range recEvs {
		if n, ok := a.sendRec(ev, now); ok {
			notes = append(notes, n)
		}
	}
	return notes, guard, nil
}

// sendSlot marks the slot notice ev as sent and starts the session it asks
// for. It reports the notice to send; false when another writer sent it, when
// the notice is silent (auto_start with notices off) or when a step failed.
// A failure is kept in RoutineErr; if the mark was written it is deleted again
// so the next tick of the window retries.
func (a *App) sendSlot(ev focus.Event, now int64) (focus.Event, bool) {
	fail := func(err error) (focus.Event, bool) {
		if a.routineErr == nil {
			a.routineErr = err
		}
		return focus.Event{}, false
	}
	// The row is the race guard: only the writer that inserted it sends.
	inserted, err := a.st.MarkSlotNotice(ev.Slot.Key, ev.SlotKind, now)
	if err != nil {
		return fail(fmt.Errorf("app: slot notice: %w", err))
	}
	if !inserted {
		return focus.Event{}, false
	}
	// The start comes after the mark: a restart can never start a second session.
	if err := a.startSlotSession(&ev); err != nil {
		if uerr := a.st.UnmarkSlotNotice(ev.Slot.Key, ev.SlotKind); uerr != nil {
			err = errors.Join(err, uerr)
		}
		return fail(err)
	}
	if ev.Silent {
		return focus.Event{}, false
	}
	ev.Kind = focus.Notify
	return ev, true
}

// startSlotSession starts the session a starting notice asks for (ev.AutoStart)
// at the times the engine chose. A session that tock started since trackd
// read the open ones (store.ErrSessionRunning) leaves ev as the plain notice,
// without the start; any other error is returned.
func (a *App) startSlotSession(ev *focus.Event) error {
	if !ev.AutoStart {
		return nil
	}
	s, err := a.st.StartSession(ev.Sess.StartMS, ev.Sess.PlannedMS)
	switch {
	case errors.Is(err, store.ErrSessionRunning):
		ev.AutoStart, ev.Body, ev.Sess = false, ev.AltBody, focus.Session{}
		return nil
	case err != nil:
		return fmt.Errorf("app: start planned session: %w", err)
	}
	ev.Sess = focus.Session(s)
	return nil
}

// ReleaseSlotNotice undoes the dedupe mark of a slot notice that GuardTick
// returned but trackd could not hand over, so the next tick of the notice's
// window sends it again. An event that is not a slot notice is ignored.
func (a *App) ReleaseSlotNotice(ev focus.Event) error {
	if ev.Slot.Key == "" || ev.SlotKind == "" {
		return nil
	}
	return a.st.UnmarkSlotNotice(ev.Slot.Key, ev.SlotKind)
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
	distract, _ := strconv.ParseInt(m[store.KeyFocusDistractSnooze], 10, 64)
	overAt, _ := strconv.ParseInt(m[store.KeyFocusOverAt], 10, 64)
	taken, _ := strconv.ParseInt(m[store.KeyFocusBreakTaken], 10, 64)
	until, _ := strconv.ParseInt(m[store.KeyFocusBreakUntil], 10, 64)
	return focus.State{BreakTakenAt: taken, BreakUntil: until, OverDay: m[store.KeyFocusOverDay], OverAt: overAt, BreakFrom: from, BreakAt: at, RecapDay: m[store.KeyFocusRecapDay], StartDay: m[store.KeyFocusStartDay], MiddayDay: m[store.KeyFocusMiddayDay], AwayAt: away,
		SnoozeUntil: snooze, DistractSnoozeUntil: distract}, nil
}

func (a *App) setFocusState(s focus.State, nowMS int64) error {
	// store.KeyFocusDistractSnooze, store.KeyFocusBreakTaken and
	// store.KeyFocusBreakUntil are not saved here: tock owns them and Step never changes them.
	for _, kv := range [][2]string{
		{store.KeyFocusBreakFrom, strconv.FormatInt(s.BreakFrom, 10)},
		{store.KeyFocusBreakAt, strconv.FormatInt(s.BreakAt, 10)},
		{store.KeyFocusRecapDay, s.RecapDay},
		{store.KeyFocusStartDay, s.StartDay},
		{store.KeyFocusMiddayDay, s.MiddayDay},
		{store.KeyFocusOverDay, s.OverDay},
		{store.KeyFocusOverAt, strconv.FormatInt(s.OverAt, 10)},
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
	s, _, err := a.live(focus.Session(*open), now)
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
// nowMS (capped at its planned end), and measures its focus level.
func (a *App) live(s focus.Session, nowMS int64) (focus.Session, focus.Live, error) {
	end := max(min(nowMS, s.PlannedMS), s.StartMS)
	vs, labels, err := a.labelled(s.StartMS, end, nowMS)
	if err != nil {
		return s, focus.Live{Level: -1}, err
	}
	ws := focus.WindowStats(vs, labels, s.StartMS, end, a.cfg.Focus)
	s.DeepMS, s.DistractMS, s.LongestMS = ws.DeepMS, ws.DistractMS, ws.LongestMS
	s.Switches = stats.WindowSwitches(vs, labels, s.StartMS, end)
	return s, focus.Level(vs, labels, s.StartMS, end, a.cfg.Focus), nil
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
	w, _, _, err := a.Weights()
	if err != nil {
		return report.Focus{}, err
	}
	f := report.Focus{NowMS: nowMS, Live: focus.Live{Level: -1}, Weights: w, SnoozeMin: int(a.cfg.Focus.SnoozeMS / 60_000)}
	for i := len(ss) - 1; i >= 0; i-- {
		s := focus.Session(ss[i])
		switch {
		case s.EndMS == 0:
			live, lv, err := a.live(s, nowMS)
			if err != nil {
				return report.Focus{}, err
			}
			f.Open, f.Live = &live, lv
			until, err := a.distractSnooze()
			if err != nil {
				return report.Focus{}, err
			}
			if until > nowMS {
				f.SnoozeUntilMS = until
			}
		case s.StartMS >= today.UnixMilli():
			f.Today = append(f.Today, s)
		default:
			f.Week = append(f.Week, s)
		}
	}
	if f.Open == nil {
		// A suggestion cannot be taken while a session runs.
		if f.Pending, err = a.PendingRecommend(now); err != nil {
			return report.Focus{}, err
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
