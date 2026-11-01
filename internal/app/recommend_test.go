package app

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/focus"
	"tock/internal/store"
)

// recEnv is a Saturday with 21 days of 09:00-10:00 Code behind it, an open
// Code span since 09:00 today, work hours on every weekday and no other
// notices, with the clock at 09:30.
func recEnv(t *testing.T, extra string) *apptest.Env {
	t.Helper()
	return recEnvCfg(t, apptest.NoRoutines+apptest.NoPlanNotices+apptest.AllHours+extra)
}

// recEnvCfg is recEnv with the whole config file given.
func recEnvCfg(t *testing.T, cfg string) *apptest.Env {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.FocusHistory(21)...)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.WriteConfig(t, cfg)
	slotClock(e, 9, 30, 0)
	return e
}

const recBody = "Your best focus hour · start a 25 min session now?"

func recRowsRaw(t *testing.T, e *apptest.Env) []store.Recommend {
	t.Helper()
	s, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rs, err := s.Recommends(0)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestRecommendNotice(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	evs, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	wantList(t, "09:30", notes(evs), wantNote(focus.TopicRecommend, "Good time to focus", recBody))
	rows := recRowsRaw(t, e)
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want 1", rows)
	}
	r := rows[0]
	if r.Kind != "focus" || r.AtMS != apptest.At(t, "09:30", 0) || r.Weekday != 6 || r.Hour != 9 || r.LengthMS != 25*60_000 || r.Reply != "" {
		t.Fatalf("row = %+v", r)
	}
	ev := evs[0]
	if ev.Minutes != 25 || ev.RecKind != "focus" || ev.RecLengthMS != 25*60_000 || ev.RecID != r.ID {
		t.Fatalf("event = %+v, want 25 min focus row %d", ev, r.ID)
	}
	wantList(t, "09:30:05", tickAt(t, a, e, 9, 30, 5))
	wantList(t, "restart 09:31", tickAt(t, open(t, e), e, 9, 31, 0))
	if n := len(recRowsRaw(t, e)); n != 1 {
		t.Fatalf("rows after more ticks = %d, want 1", n)
	}
}

func TestRecommendOff(t *testing.T) {
	for name, extra := range map[string]string{
		"disabled":   "[recommend]\nenabled = false\n",
		"max 0":      "[recommend]\nmax_per_day = 0\n",
		"no notices": "[focus]\nnotify = false\n",
	} {
		e := recEnv(t, extra)
		a := open(t, e)
		wantList(t, name, tickAt(t, a, e, 9, 30, 0))
		if rows := recRowsRaw(t, e); len(rows) != 0 {
			t.Errorf("%s: rows = %+v, want none", name, rows)
		}
	}
	// Six days of history is thin.
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.FocusHistory(6)...)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices+apptest.AllHours)
	a := open(t, e)
	wantList(t, "thin", tickAt(t, a, e, 9, 30, 0))
	if rows := recRowsRaw(t, e); len(rows) != 0 {
		t.Errorf("thin: rows = %+v", rows)
	}
}

func TestAnswerRecommend(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	evs, err := a.FocusTick(false)
	if err != nil || len(evs) != 1 {
		t.Fatalf("tick = %v, %v", evs, err)
	}
	id := evs[0].RecID
	res, err := a.AnswerRecommend(id, "focus", "accept")
	if err != nil {
		t.Fatal(err)
	}
	now := apptest.At(t, "09:30", 0)
	if res.Rec.Reply != "accept" || res.Session == nil || res.Session.PlannedMS != now+25*60_000 {
		t.Fatalf("answer = %+v / %+v", res.Rec, res.Session)
	}
	if s, err := a.st.OpenSession(); err != nil || s == nil || s.PlannedMS != now+25*60_000 {
		t.Fatalf("open session = %+v, %v, want the 25 min session running", s, err)
	}
	if r, _ := a.st.Recommend(id); r.Reply != "accept" || r.ReplyMS != now {
		t.Fatalf("row = %+v", r)
	}
	if _, err := a.AnswerRecommend(id, "focus", "accept"); !errors.Is(err, ErrNoRecommend) {
		t.Fatalf("second answer err = %v, want ErrNoRecommend", err)
	}
	if _, err := a.AnswerRecommend(id, "", "bogus"); err == nil || errors.Is(err, ErrNoRecommend) {
		t.Fatalf("bogus reply err = %v, want a plain error", err)
	}

	// A fresh row: dismiss starts nothing and takes any kind.
	add := func(at int64) int64 {
		t.Helper()
		id, err := a.st.AddRecommend(store.Recommend{Kind: "focus", AtMS: at, Weekday: 6, Hour: 9, LengthMS: 10 * 60_000})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	b := add(now - 60_000)
	res, err = a.AnswerRecommend(b, "", "dismiss")
	if err != nil || res.Rec.Reply != "dismiss" || res.Session != nil {
		t.Fatalf("dismiss = %+v, %v", res, err)
	}
	// Replies 30 min or more after the row are refused.
	c := add(now - 30*60_000)
	if _, err := a.AnswerRecommend(c, "focus", "dismiss"); !errors.Is(err, ErrNoRecommend) {
		t.Fatalf("30 min old err = %v, want ErrNoRecommend", err)
	}
	// A break kind for a focus row.
	d := add(now - 60_000)
	if _, err := a.AnswerRecommend(d, "break", "dismiss"); !errors.Is(err, ErrNoRecommend) {
		t.Fatalf("wrong kind err = %v, want ErrNoRecommend", err)
	}
	if r, _ := a.st.Recommend(d); r.Reply != "" {
		t.Fatalf("row changed: %+v", r)
	}
	// Accept while a session runs: error, reply back to ''.
	if _, err := a.AnswerRecommend(d, "focus", "accept"); !errors.Is(err, store.ErrSessionRunning) {
		t.Fatalf("accept with a session running err = %v, want ErrSessionRunning", err)
	}
	if r, _ := a.st.Recommend(d); r.Reply != "" || r.ReplyMS != 0 {
		t.Fatalf("row after failed accept = %+v, want unanswered", r)
	}
	if _, err := a.AnswerRecommend(999, "", "dismiss"); !errors.Is(err, ErrNoRecommend) {
		t.Fatalf("unknown id err = %v", err)
	}
}

func TestAnswerRecommendBreakUnsupported(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	id, err := a.st.AddRecommend(store.Recommend{Kind: "break", AtMS: e.Clock.UnixMilli() - 1000, LengthMS: 10 * 60_000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.AnswerRecommend(id, "break", "accept"); err == nil || errors.Is(err, ErrNoRecommend) {
		t.Fatalf("accept of a break err = %v, want a plain error", err)
	}
	if r, _ := a.st.Recommend(id); r.Reply != "" {
		t.Fatalf("row = %+v, want unchanged", r)
	}
}

func TestProfileCachedPerDay(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	p, err := a.Profile(e.Clock)
	if err != nil || len(p.Top[time.Saturday]) != 1 || p.Top[time.Saturday][0] != 9 {
		t.Fatalf("Profile = %+v, %v, want Top[Saturday] [9]", p.Top, err)
	}
	db := e.RawDB(t)
	if _, err := db.Exec(`DELETE FROM span`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	slotClock(e, 11, 0, 0)
	p, err = a.Profile(e.Clock)
	if err != nil || p.Rank(time.Saturday, 9) != 1 {
		t.Fatalf("same day after deleting history: %+v, %v, want the cached profile", p.Top, err)
	}
	e.Clock = e.Clock.AddDate(0, 0, 1)
	if _, err := a.Profile(e.Clock); err != nil || a.profile.day != "2026-10-11" {
		t.Fatalf("next day: cache day %q, err %v, want rebuilt for 2026-10-11", a.profile.day, err)
	}
	e.Clock = e.Clock.AddDate(0, 0, -1)
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices+apptest.AllHours+"# changed\n")
	if _, err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	p, err = a.Profile(e.Clock)
	if err != nil || !p.Thin {
		t.Fatalf("after a config change: %+v, %v, want a rebuilt, thin profile", p, err)
	}
}

// TestRecommendIdleTickReadsNoRows: trackd's idle budget. Outside a top hour
// no recommendation row is read; the profile is built once.
func TestRecommendIdleTickReadsNoRows(t *testing.T) {
	e := recEnv(t, "[focus]\nbreak_after = 480\n")
	a := open(t, e)
	wantList(t, "11:00", tickAt(t, a, e, 11, 0, 0))
	if a.recCache.ok {
		t.Error("rows were read on a tick where no suggestion could be due")
	}
	if a.profile.p == nil {
		t.Error("profile was not built")
	}
	p := a.profile.p
	wantList(t, "11:00:05", tickAt(t, a, e, 11, 0, 5))
	if a.profile.p != p {
		t.Error("profile was rebuilt within the day")
	}
	if got := tickAt(t, a, e, 12, 0, 0); len(got) != 0 || a.recCache.ok {
		t.Error("rows read at 12:00")
	}
	tickAt(t, a, e, 9, 30, 0)
	if !a.recCache.ok {
		t.Error("rows were not read on a tick where a suggestion was due")
	}
}

// TestRecommendSlots: a plan item soon caps the suggested length; one that is
// too close, or running, suppresses it. Plan notices stay off throughout.
func TestRecommendSlots(t *testing.T) {
	for _, c := range []struct {
		item string
		want string // body length, "" = no notice
		rows int
	}{
		{"Review @09h50 due:2026-10-10\n", "20", 1},
		{"Review @09h40 due:2026-10-10\n", "", 0},
		{"Review @09h00 due:2026-10-10\n", "", 0},
	} {
		todo := filepath.Join(t.TempDir(), "todo.txt")
		writePlan(t, todo, c.item)
		e := recEnvCfg(t, apptest.NoRoutines+apptest.AllHours+"[plan]\nfile = '"+todo+"'\nnotify = false\n")
		a := open(t, e)
		evs, err := a.FocusTick(false)
		if err != nil {
			t.Fatal(err)
		}
		if c.want == "" {
			wantList(t, c.item, notes(evs))
		} else {
			wantList(t, c.item, notes(evs), wantNote(focus.TopicRecommend, "Good time to focus",
				"Your best focus hour · start a "+c.want+" min session now?"))
			if evs[0].Minutes != 20 {
				t.Errorf("Minutes = %d, want 20", evs[0].Minutes)
			}
		}
		rows := recRowsRaw(t, e)
		if len(rows) != c.rows || (c.rows == 1 && rows[0].LengthMS != 20*60_000) {
			t.Errorf("%q: rows = %+v, want %d", c.item, rows, c.rows)
		}
	}
}

// TestRecRowsThrottled: the rows are read once a minute, and a row trackd
// wrote is in the cache at once.
func TestRecRowsThrottled(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	if got := tickAt(t, a, e, 9, 30, 0); len(got) != 1 {
		t.Fatalf("09:30 = %q", got)
	}
	db := e.RawDB(t)
	if _, err := db.Exec(`DELETE FROM recommend`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	wantList(t, "09:30:40", tickAt(t, a, e, 9, 30, 40))
	if got := tickAt(t, a, e, 9, 31, 5); len(got) != 1 {
		t.Fatalf("09:31:05 = %q, want the suggestion again once the cache is a minute old", got)
	}
}

// TestReleaseRecommend: a suggestion trackd could not hand over is deleted, so
// the next tick sends it again; an answered one stays.
func TestReleaseRecommend(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	evs, err := a.FocusTick(false)
	if err != nil || len(evs) != 1 {
		t.Fatalf("tick = %v, %v", evs, err)
	}
	if err := a.ReleaseRecommend(focus.Event{}); err != nil {
		t.Fatal(err)
	}
	if n := len(recRowsRaw(t, e)); n != 1 {
		t.Fatalf("rows after releasing an event without a row = %d, want 1", n)
	}
	if err := a.ReleaseRecommend(evs[0]); err != nil {
		t.Fatal(err)
	}
	if n := len(recRowsRaw(t, e)); n != 0 {
		t.Fatalf("rows after release = %d, want 0", n)
	}
	again, err := a.FocusTick(false)
	if err != nil || len(again) != 1 {
		t.Fatalf("tick after release = %v, %v, want a new suggestion", again, err)
	}
	if _, err := a.AnswerRecommend(again[0].RecID, "", "dismiss"); err != nil {
		t.Fatal(err)
	}
	if err := a.ReleaseRecommend(again[0]); err != nil {
		t.Fatal(err)
	}
	if n := len(recRowsRaw(t, e)); n != 1 {
		t.Fatalf("an answered row was deleted: rows = %d", n)
	}
}

func TestPendingRecommend(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	now := e.Clock
	nowMS := now.UnixMilli()
	if r, err := a.PendingRecommend(now); err != nil || r != nil {
		t.Fatalf("no rows: %+v, %v, want nil", r, err)
	}
	add := func(at int64) int64 {
		t.Helper()
		id, err := a.st.AddRecommend(store.Recommend{Kind: "focus", AtMS: at, Weekday: 6, Hour: 9, LengthMS: 25 * 60_000})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	old := add(nowMS - 10*60_000)
	r, err := a.PendingRecommend(now)
	if err != nil || r == nil || r.ID != old || r.Kind != "focus" || r.LengthMS != 25*60_000 {
		t.Fatalf("10 min old: %+v, %v", r, err)
	}
	newer := add(nowMS - 5*60_000)
	if r, err := a.PendingRecommend(now); err != nil || r == nil || r.ID != newer {
		t.Fatalf("two open: %+v, %v, want the newer", r, err)
	}
	if _, err := a.AnswerRecommend(newer, "", "dismiss"); err != nil {
		t.Fatal(err)
	}
	if r, err := a.PendingRecommend(now); err != nil || r == nil || r.ID != old {
		t.Fatalf("after answering the newer: %+v, %v, want the older", r, err)
	}
	if _, err := a.AnswerRecommend(old, "", "accept"); err != nil {
		t.Fatal(err)
	}
	if r, err := a.PendingRecommend(now); err != nil || r != nil {
		t.Fatalf("answered: %+v, %v, want nil", r, err)
	}
	if r, err := a.PendingRecommend(now.Add(31 * time.Minute)); err != nil || r != nil {
		t.Fatalf("expired: %+v, %v, want nil", r, err)
	}
}

func TestPendingRecommendExpiry(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	nowMS := e.Clock.UnixMilli()
	if _, err := a.st.AddRecommend(store.Recommend{Kind: "focus", AtMS: nowMS - 30*60_000, LengthMS: 25 * 60_000}); err != nil {
		t.Fatal(err)
	}
	if r, err := a.PendingRecommend(e.Clock); err != nil || r != nil {
		t.Fatalf("30 min old: %+v, %v, want nil", r, err)
	}
}

// TestAnswerRecommendAt: ids are reused after a delete (no AUTOINCREMENT), so
// a surface that showed the row names its send time too, and a newer row that
// took the same id is not answered by the older view.
func TestAnswerRecommendAt(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	nowMS := e.Clock.UnixMilli()
	first, err := a.st.AddRecommend(store.Recommend{Kind: "focus", AtMS: nowMS - 20*60_000, LengthMS: 25 * 60_000})
	if err != nil {
		t.Fatal(err)
	}
	shownAt := nowMS - 20*60_000
	if err := a.st.DeleteRecommend(first); err != nil {
		t.Fatal(err)
	}
	second, err := a.st.AddRecommend(store.Recommend{Kind: "focus", AtMS: nowMS - 2*60_000, LengthMS: 10 * 60_000})
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Skipf("sqlite did not reuse id %d (got %d)", first, second)
	}
	if _, err := a.AnswerRecommendAt(first, shownAt, "", "accept"); !errors.Is(err, ErrNoRecommend) {
		t.Fatalf("stale view err = %v, want ErrNoRecommend", err)
	}
	if r, _ := a.st.Recommend(second); r.Reply != "" {
		t.Fatalf("newer row answered by a stale view: %+v", r)
	}
	if s, _ := a.st.OpenSession(); s != nil {
		t.Fatalf("a session started: %+v", s)
	}
	if _, err := a.AnswerRecommendAt(second, nowMS-2*60_000, "", "dismiss"); err != nil {
		t.Fatalf("matching view err = %v", err)
	}
}

// TestFocusPending: report.Focus carries the open recommendation only while
// no session runs.
func TestFocusPending(t *testing.T) {
	e := recEnv(t, "")
	a := open(t, e)
	nowMS := e.Clock.UnixMilli()
	id, err := a.st.AddRecommend(store.Recommend{Kind: "focus", AtMS: nowMS - 5*60_000, LengthMS: 25 * 60_000})
	if err != nil {
		t.Fatal(err)
	}
	f, err := a.Focus(e.Clock)
	if err != nil || f.Pending == nil || f.Pending.ID != id {
		t.Fatalf("Pending = %+v, %v, want row %d", f.Pending, err, id)
	}
	if _, err := a.StartSession(10); err != nil {
		t.Fatal(err)
	}
	if f, err = a.Focus(e.Clock); err != nil || f.Pending != nil {
		t.Fatalf("with a session running: Pending = %+v, %v, want nil", f.Pending, err)
	}
}
