package app

import (
	"errors"
	"fmt"
	"slices"
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

// profileRetry is how soon a failed profile build is tried again.
const profileRetry = time.Minute

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

// recInput fills in the recommendation part of a tick's input. The profile
// comes from the daily cache; the rows are read only when focus.RecsDue says a
// suggestion could be due. A failure is kept as the tick's RoutineErr (when no
// other is set) and leaves the tick without a suggestion.
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
// notice to send, carrying the row's id. It reports false, and sends nothing,
// when the row could not be written; the error is kept in RoutineErr and the
// next tick suggests again.
func (a *App) sendRec(ev focus.Event, now int64) (focus.Event, bool) {
	t := time.UnixMilli(now).In(a.now().Location())
	r := store.Recommend{Kind: ev.RecKind, AtMS: now, Weekday: int(t.Weekday()), Hour: t.Hour(), LengthMS: ev.RecLengthMS}
	id, err := a.st.AddRecommend(r)
	if err != nil {
		if a.routineErr == nil {
			a.routineErr = fmt.Errorf("app: recommendation: %w", err)
		}
		return focus.Event{}, false
	}
	r.ID = id
	if a.recCache.ok {
		a.recCache.rows = append(a.recCache.rows, recommend.Rec(r))
	}
	ev.RecID = id
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

// RecAnswer is what answering a recommendation did: the row as it is now and,
// for an accepted focus suggestion, the session that was started.
type RecAnswer struct {
	Rec     recommend.Rec
	Session *focus.Session
}

// AnswerRecommend records reply (recommend.ReplyAccept or ReplyDismiss) to the
// recommendation id, once, and for an accepted focus suggestion starts a manual
// session of the length the row holds. kind "" accepts any kind; otherwise it
// must be the row's. The row must still be open (unanswered, under
// recommend.PendingMS old), else ErrNoRecommend. If the session cannot start
// (store.ErrSessionRunning) the reply goes back to ” and the error is
// returned, so the suggestion stays open to dismiss.
func (a *App) AnswerRecommend(id int64, kind, reply string) (RecAnswer, error) {
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
	if (kind != "" && rec.Kind != kind) || !rec.Open(now) {
		return RecAnswer{}, ErrNoRecommend
	}
	if reply == recommend.ReplyAccept && rec.Kind != recommend.KindFocus {
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
