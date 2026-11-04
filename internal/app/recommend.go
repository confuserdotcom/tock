package app

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"tock/internal/focus"
	"tock/internal/recommend"
	"tock/internal/store"
)

// ErrNoRecommend: the recommendation is not open: unknown, answered, older
// than recommend.PendingMS or of another kind.
var ErrNoRecommend = errors.New("no pending recommendation")

// recRefresh is how often the recommendation rows are read again: not on every
// 5 s tick. A row trackd writes itself goes into the cache at once, so the gap
// and the daily count hold within the minute.
const recRefresh = time.Minute

// profileRetry is how soon a failed profile build is tried again: a full
// 28-day recompute every tick would eat the idle budget of trackd and repeat the
// same warning for as long as the cause lasts.
const profileRetry = 10 * time.Minute

// recCache holds the recommendation rows of the last recommend.ReplyDays.
type recCache struct {
	ok        bool
	checkedMS int64
	rows      []recommend.Rec
	// failedMS is the tick time (ms) of the last failed profile build, 0 = none.
	failedMS int64
}

// recRows returns the recommendation rows of the last recommend.ReplyDays,
// read at most once per recRefresh.
func (a *App) recRows(now time.Time) ([]recommend.Rec, error) {
	nowMS := now.UnixMilli()
	c := &a.recCache
	if age := nowMS - c.checkedMS; c.ok && age >= 0 && age < recRefresh.Milliseconds() {
		return slices.Clone(c.rows), nil
	}
	rows, err := a.st.Recommends(nowMS - recommend.ReplyDays*recommend.DayMS)
	if err != nil {
		return nil, err
	}
	out := make([]recommend.Rec, len(rows))
	for i, r := range rows {
		out[i] = recommend.Rec(r)
	}
	c.ok, c.checkedMS, c.rows = true, nowMS, out
	return slices.Clone(out), nil
}

// profileUsed reports that a tick reads the profile: focus suggestions can come
// (max_per_day above 0, [focus] notify on and [hours] set) or the break reminder
// follows it (adaptive_break). Otherwise trackd does not build it.
func (a *App) profileUsed() bool {
	rc := a.cfg.Recommend
	return rc.Enabled && (rc.AdaptiveBreak || rc.MaxPerDay > 0 && a.cfg.Focus.Notify && a.cfg.Hours.Set)
}

// recInput fills in the recommendation part of a tick's input. The profile
// comes from the daily cache, and is not built when profileUsed says nothing
// reads it; the rows are read only when focus.RecsDue says a suggestion could
// be due. A failure is kept as the tick's RoutineErr (when no other is set) and
// leaves the tick without a suggestion.
func (a *App) recInput(in *focus.Input, nowT time.Time) {
	in.Rec = a.cfg.Recommend
	if !in.Rec.Enabled {
		return
	}
	fail := func(err error) {
		if a.routineErr == nil {
			a.routineErr = err
		}
	}
	nowMS := nowT.UnixMilli()
	if a.profileUsed() {
		if a.profile.p == nil || a.profile.day != dateKey(in.Day) {
			if f := a.recCache.failedMS; f != 0 && nowMS-f >= 0 && nowMS-f < profileRetry.Milliseconds() {
				return
			}
			if _, err := a.Profile(nowT); err != nil {
				a.recCache.failedMS = nowMS
				fail(err)
				return
			}
			a.recCache.failedMS = 0
		}
		in.Profile = *a.profile.p
	}
	if !focus.RecsDue(*in) {
		return
	}
	// The free-moment rule reads its own copy of today's slots from the
	// throttled cache, so it works with plan notices off and never disturbs
	// the slot notices.
	slots, err := a.Slots(nowT)
	if err != nil {
		fail(fmt.Errorf("app: recommendation slots: %w", err))
		return
	}
	rows, err := a.recRows(nowT)
	if err != nil {
		fail(fmt.Errorf("app: recommendations: %w", err))
		return
	}
	in.RecSlots, in.Recs, in.RecsOK = slots, rows, true
}

// sendRec writes the recommend row of the RecDue event ev and returns the
// notice to send, carrying the row's id. For a focus suggestion it reports
// false, and sends nothing, when the row could not be written; the error is
// kept in RoutineErr and the next tick suggests again. A break reminder is
// sent anyway, as the plain reminder without a row (and without the Take a
// break button): the reminder is already marked sent in the focus state.
func (a *App) sendRec(ev focus.Event, now int64) (focus.Event, bool) {
	t := time.UnixMilli(now).In(a.now().Location())
	r := store.Recommend{Kind: ev.RecKind, AtMS: now, Weekday: int(t.Weekday()), Hour: t.Hour(), LengthMS: ev.RecLengthMS}
	id, err := a.st.AddRecommend(r)
	if err != nil {
		if a.routineErr == nil {
			a.routineErr = fmt.Errorf("app: recommendation: %w", err)
		}
		if ev.RecKind == recommend.KindBreak {
			ev.Kind, ev.Minutes, ev.RecKind, ev.RecLengthMS = focus.Notify, 0, "", 0
			return ev, true
		}
		return focus.Event{}, false
	}
	r.ID = id
	if a.recCache.ok {
		a.recCache.rows = append(a.recCache.rows, recommend.Rec(r))
	}
	ev.RecID, ev.RecAtMS = id, now
	ev.Kind = focus.Notify
	return ev, true
}

// ReleaseRecommend deletes the unanswered row of a recommendation notice that
// GuardTick returned but trackd could not hand over, so the next tick suggests
// again. An event without a row is ignored.
func (a *App) ReleaseRecommend(ev focus.Event) error {
	if ev.RecID == 0 {
		return nil
	}
	if err := a.st.DeleteRecommend(ev.RecID); err != nil {
		return err
	}
	a.recCache.rows = slices.DeleteFunc(a.recCache.rows, func(r recommend.Rec) bool { return r.ID == ev.RecID && r.Reply == "" })
	return nil
}

// ReleaseNudge gives back the budget a nudge GuardTick returned took, when
// trackd could not hand it over: the count drops by one and the gap is
// lifted. Any other notice is ignored.
func (a *App) ReleaseNudge(ev focus.Event) error {
	if !focus.IsNudge(ev.Topic) {
		return nil
	}
	st, err := a.focusState()
	if err != nil {
		return err
	}
	if st.Nudges == 0 && st.NudgeAt == 0 {
		return nil
	}
	st.Nudges, st.NudgeAt = max(st.Nudges-1, 0), 0
	return a.setFocusState(st, a.nowMS())
}

// PendingRecommend returns the newest recommendation that is still open at
// now (unanswered, sent under recommend.PendingMS ago), nil if none. One
// indexed read of the last 30 minutes of rows.
func (a *App) PendingRecommend(now time.Time) (*recommend.Rec, error) {
	nowMS := now.UnixMilli()
	rows, err := a.st.Recommends(nowMS - recommend.PendingMS)
	if err != nil {
		return nil, fmt.Errorf("app: pending recommendation: %w", err)
	}
	recs := make([]recommend.Rec, len(rows))
	for i, r := range rows {
		recs[i] = recommend.Rec(r)
	}
	r := recommend.Pending(recs, nowMS)
	if r == nil {
		return nil, nil
	}
	out := *r
	return &out, nil
}

// RecAnswer is what answering a recommendation did: the row as it is now and,
// for an accepted focus suggestion, the session that was started.
type RecAnswer struct {
	Rec     recommend.Rec
	Session *focus.Session
	// BreakUntilMS (and BreakUntil, in the App clock's zone) is when the break
	// ends, for an accepted break; 0 otherwise.
	BreakUntilMS int64
	BreakUntil   time.Time
}

// AnswerRecommend records reply (recommend.ReplyAccept or ReplyDismiss) to the
// recommendation id, once, and for an accepted focus suggestion starts a manual
// session of the length the row holds. kind "" accepts any kind; otherwise it
// must be the row's. The row must still be open (unanswered, under
// recommend.PendingMS old), else ErrNoRecommend. If the session cannot start
// (store.ErrSessionRunning) the reply goes back to ” and the error is
// returned, so the suggestion stays open to dismiss.
func (a *App) AnswerRecommend(id int64, kind, reply string) (RecAnswer, error) {
	return a.AnswerRecommendAt(id, 0, kind, reply)
}

// AnswerRecommendAt is AnswerRecommend for a surface that showed the row: atMS
// is the send time it showed. Row ids can be reused (no AUTOINCREMENT: a
// released row's id goes to the next one), so a stale menu or screen must not
// answer a newer row that took the same id: with atMS > 0 a row sent at
// another time is ErrNoRecommend. atMS 0 matches any row.
func (a *App) AnswerRecommendAt(id, atMS int64, kind, reply string) (RecAnswer, error) {
	if reply != recommend.ReplyAccept && reply != recommend.ReplyDismiss {
		return RecAnswer{}, fmt.Errorf("app: recommend: reply %q, want %s or %s", reply, recommend.ReplyAccept, recommend.ReplyDismiss)
	}
	row, err := a.st.Recommend(id)
	if errors.Is(err, store.ErrRecommendGone) {
		return RecAnswer{}, ErrNoRecommend
	}
	if err != nil {
		return RecAnswer{}, fmt.Errorf("app: recommend: %w", err)
	}
	now := a.nowMS()
	rec := recommend.Rec(row)
	if (kind != "" && rec.Kind != kind) || (atMS > 0 && rec.AtMS != atMS) || !rec.Open(now) {
		return RecAnswer{}, ErrNoRecommend
	}
	if reply == recommend.ReplyAccept && rec.Kind != recommend.KindFocus && rec.Kind != recommend.KindBreak {
		return RecAnswer{}, fmt.Errorf("app: recommend: accepting a %s recommendation is not supported", rec.Kind)
	}
	ok, err := a.st.ReplyRecommend(id, reply, now, now-recommend.PendingMS)
	if err != nil {
		return RecAnswer{}, fmt.Errorf("app: recommend: %w", err)
	}
	if !ok {
		return RecAnswer{}, ErrNoRecommend
	}
	a.recCache = recCache{failedMS: a.recCache.failedMS}
	rec.Reply, rec.ReplyMS = reply, now
	res := RecAnswer{Rec: rec}
	if reply == recommend.ReplyAccept && rec.Kind == recommend.KindBreak {
		until, err := a.TakeBreak(rec.LengthMS)
		if err != nil {
			if uerr := a.st.UnreplyRecommend(id, reply); uerr != nil {
				err = errors.Join(err, uerr)
			}
			return RecAnswer{}, err
		}
		res.BreakUntilMS, res.BreakUntil = until, time.UnixMilli(until).In(a.now().Location())
		return res, nil
	}
	if reply == recommend.ReplyAccept {
		s, err := a.StartSession(int(rec.LengthMS / 60_000))
		if err != nil {
			if uerr := a.st.UnreplyRecommend(id, reply); uerr != nil {
				err = errors.Join(err, uerr)
			}
			return RecAnswer{}, err
		}
		res.Session = &s
	}
	return res, nil
}

// SnoozeRecommend closes the break reminder row id, sent at atMS (0 = any),
// with the reply recommend.ReplySnooze: the user snoozed the reminders, so the
// suggestion is no longer offered, but it is no answer to count: a snooze never
// mutes an hour. A row that is not an open break reminder is ErrNoRecommend.
func (a *App) SnoozeRecommend(id, atMS int64) error {
	row, err := a.st.Recommend(id)
	if errors.Is(err, store.ErrRecommendGone) {
		return ErrNoRecommend
	}
	if err != nil {
		return fmt.Errorf("app: recommend: %w", err)
	}
	now := a.nowMS()
	rec := recommend.Rec(row)
	if rec.Kind != recommend.KindBreak || (atMS > 0 && rec.AtMS != atMS) || !rec.Open(now) {
		return ErrNoRecommend
	}
	ok, err := a.st.ReplyRecommend(id, recommend.ReplySnooze, now, now-recommend.PendingMS)
	if err != nil {
		return fmt.Errorf("app: recommend: %w", err)
	}
	if !ok {
		return ErrNoRecommend
	}
	a.recCache = recCache{failedMS: a.recCache.failedMS}
	return nil
}

// TakeBreak starts the break the user took from a break reminder: break
// reminders are held back for lengthMS from now, and when the break ends
// trackd sends one Break over notice. Nothing is locked or blocked. It
// returns when the break ends (ms). tock owns the two keys it writes; trackd
// only reads them, so a take between trackd's read and write is never lost.
func (a *App) TakeBreak(lengthMS int64) (untilMS int64, err error) {
	if lengthMS <= 0 {
		return 0, fmt.Errorf("app: take break: length %d ms, want more than 0", lengthMS)
	}
	now := a.nowMS()
	untilMS = now + lengthMS
	// Taken first: a trackd that reads between the two writes sees the old end.
	if err := a.st.SetState(store.KeyFocusBreakTaken, strconv.FormatInt(now, 10), now); err != nil {
		return 0, fmt.Errorf("app: take break: %w", err)
	}
	if err := a.st.SetState(store.KeyFocusBreakUntil, strconv.FormatInt(untilMS, 10), now); err != nil {
		return 0, fmt.Errorf("app: take break: %w", err)
	}
	return untilMS, nil
}

// BreakUntil is when the break the user took ends (ms), 0 when none runs at
// now.
func (a *App) BreakUntil(now time.Time) (int64, error) {
	v, ok, err := a.st.State(store.KeyFocusBreakUntil)
	if err != nil {
		return 0, fmt.Errorf("app: break until: %w", err)
	}
	if !ok {
		return 0, nil
	}
	until, _ := strconv.ParseInt(v, 10, 64)
	if until <= now.UnixMilli() {
		return 0, nil
	}
	return until, nil
}

// Learning is what the replies of the last recommend.ReplyDays say at now (the
// rows are cached for a minute, and a reply drops the cache).
func (a *App) Learning(now time.Time) (recommend.Learning, error) {
	rows, err := a.recRows(now)
	if err != nil {
		return recommend.Learning{}, fmt.Errorf("app: learning: %w", err)
	}
	return recommend.Learn(rows, now.UnixMilli()), nil
}
