package app

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"tock/internal/apptest"
	"tock/internal/focus"
	"tock/internal/store"
)

// breakEnv is a Saturday with days past days of a 52 min terminal run each
// (08:00-08:52), an open terminal span since 09:00 today, no [hours] (so no
// focus suggestions), no routine or plan notices, and the clock at 09:30.
func breakEnv(t *testing.T, days int, extra string) *apptest.Env {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	var hist []apptest.Span
	for i := 1; i <= days; i++ {
		hist = append(hist, apptest.Span{ID: int64(2000 + i), Start: "08:00", End: "08:52", Bundle: "term", App: "WezTerm",
			Title: "tock │ h", Day: -i})
	}
	e.Seed(t, hist...)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices+extra)
	slotClock(e, 9, 30, 0)
	return e
}

// breakTick ticks at h:m:s and returns the break reminders it produced.
func breakTick(t *testing.T, a *App, e *apptest.Env, h, m, s int) []focus.Event {
	t.Helper()
	slotClock(e, h, m, s)
	evs, err := a.FocusTick(false)
	if err != nil {
		t.Fatal(err)
	}
	var out []focus.Event
	for _, ev := range evs {
		if ev.Topic == focus.TopicBreak {
			out = append(out, ev)
		}
	}
	return out
}

func wantBreak(t *testing.T, label string, got []focus.Event, body string) {
	t.Helper()
	if body == "" {
		if len(got) != 0 {
			t.Fatalf("%s: got %+v, want no break reminder", label, got)
		}
		return
	}
	if len(got) != 1 || got[0].Title != focus.TitleBreak || got[0].Body != body {
		t.Fatalf("%s: got %+v, want one break reminder %q", label, got, body)
	}
}

func TestAdaptiveBreakFromHistory(t *testing.T) {
	e := breakEnv(t, 10, "")
	a := open(t, e)
	wantBreak(t, "09:51", breakTick(t, a, e, 9, 51, 0), "")
	wantBreak(t, "09:52", breakTick(t, a, e, 9, 52, 0), "52 min without a break. Stand up for 10 min.")

	// adaptive_break off: the [focus] break_after of 50 min.
	e = breakEnv(t, 10, "[recommend]\nadaptive_break = false\n")
	a = open(t, e)
	wantBreak(t, "off 09:50", breakTick(t, a, e, 9, 50, 0), "50 min without a break. Stand up for 5 min.")

	// Five days is too little history for a sustained length.
	e = breakEnv(t, 5, "")
	a = open(t, e)
	wantBreak(t, "thin 09:50", breakTick(t, a, e, 9, 50, 0), "50 min without a break. Stand up for 5 min.")
}

// breakRows are the recommend rows of kind break.
func breakRows(t *testing.T, e *apptest.Env) []store.Recommend {
	t.Helper()
	var out []store.Recommend
	for _, r := range recRowsRaw(t, e) {
		if r.Kind == "break" {
			out = append(out, r)
		}
	}
	return out
}

func stateVal(t *testing.T, e *apptest.Env, key string) string {
	t.Helper()
	db := e.RawDB(t)
	defer db.Close()
	var v string
	if err := db.QueryRow(`SELECT value FROM daemon_state WHERE key=?`, key).Scan(&v); err != nil {
		return ""
	}
	return v
}

func TestBreakRecommend(t *testing.T) {
	e := breakEnv(t, 10, "")
	a := open(t, e)
	got := breakTick(t, a, e, 9, 52, 0)
	wantBreak(t, "09:52", got, "52 min without a break. Stand up for 10 min.")
	ev := got[0]
	if ev.Kind != focus.Notify || ev.Minutes != 10 {
		t.Fatalf("event = %+v, want a Notify of 10 min", ev)
	}
	rows := breakRows(t, e)
	if len(rows) != 1 {
		t.Fatalf("break rows = %+v, want 1", rows)
	}
	r := rows[0]
	if ev.RecID != r.ID || r.AtMS != apptest.At(t, "09:52", 0) || r.Hour != 9 || r.Weekday != 6 || r.LengthMS != 10*60_000 || r.Reply != "" {
		t.Fatalf("row = %+v, event RecID %d", r, ev.RecID)
	}
}

func TestTakeBreak(t *testing.T) {
	e := breakEnv(t, 10, "")
	a := open(t, e)
	ev := breakTick(t, a, e, 9, 52, 0)[0]
	slotClock(e, 9, 53, 0)
	res, err := a.AnswerRecommend(ev.RecID, "break", "accept")
	if err != nil {
		t.Fatal(err)
	}
	until := apptest.At(t, "10:03", 0)
	if res.BreakUntilMS != until || res.Session != nil || res.Rec.Reply != "accept" {
		t.Fatalf("answer = %+v, want a break until 10:03", res)
	}
	if got := stateVal(t, e, store.KeyFocusBreakTaken); got != strconv.FormatInt(apptest.At(t, "09:53", 0), 10) {
		t.Fatalf("focus_break_taken = %q", got)
	}
	if got := stateVal(t, e, store.KeyFocusBreakUntil); got != strconv.FormatInt(until, 10) {
		t.Fatalf("focus_break_until = %q", got)
	}
	wantBreak(t, "09:55", breakTick(t, a, e, 9, 55, 0), "")
	wantBreak(t, "10:02", breakTick(t, a, e, 10, 2, 0), "")
	// The tick wrote its own state but not the break keys.
	if got := stateVal(t, e, store.KeyFocusBreakUntil); got != strconv.FormatInt(until, 10) {
		t.Fatalf("a tick rewrote focus_break_until: %q", got)
	}
	if got := stateVal(t, e, store.KeyFocusBreakTaken); got != strconv.FormatInt(apptest.At(t, "09:53", 0), 10) {
		t.Fatalf("a tick rewrote focus_break_taken: %q", got)
	}
	if got, err := a.BreakUntil(time.UnixMilli(apptest.At(t, "09:55", 0))); err != nil || got != until {
		t.Fatalf("BreakUntil(09:55) = %d, %v, want %d", got, err, until)
	}
	if got, err := a.BreakUntil(time.UnixMilli(apptest.At(t, "10:04", 0))); err != nil || got != 0 {
		t.Fatalf("BreakUntil(10:04) = %d, %v, want 0", got, err)
	}

	// Not now: stored, and the repeat comes after break_repeat as before.
	e = breakEnv(t, 10, "")
	a = open(t, e)
	ev = breakTick(t, a, e, 9, 52, 0)[0]
	slotClock(e, 9, 53, 0)
	res, err = a.AnswerRecommend(ev.RecID, "break", "dismiss")
	if err != nil || res.Rec.Reply != "dismiss" || res.BreakUntilMS != 0 {
		t.Fatalf("dismiss = %+v, %v", res, err)
	}
	if got := stateVal(t, e, store.KeyFocusBreakUntil); got != "" {
		t.Fatalf("dismiss wrote focus_break_until %q", got)
	}
	wantBreak(t, "09:59", breakTick(t, a, e, 9, 59, 0), "")
	wantBreak(t, "10:02", breakTick(t, a, e, 10, 2, 0), "62 min without a break. Stand up for 10 min.")
}

// A notice of one kind cannot answer a row of the other.
func TestAnswerBreakKindChecked(t *testing.T) {
	e := breakEnv(t, 10, "")
	a := open(t, e)
	ev := breakTick(t, a, e, 9, 52, 0)[0]
	if _, err := a.AnswerRecommend(ev.RecID, "focus", "accept"); !errors.Is(err, ErrNoRecommend) {
		t.Fatalf("focus answer to a break row = %v, want ErrNoRecommend", err)
	}
	if got := stateVal(t, e, store.KeyFocusBreakUntil); got != "" {
		t.Fatalf("wrong kind started a break: %q", got)
	}
}

// A break row that cannot be written still brings the plain reminder.
func TestBreakRowFailure(t *testing.T) {
	e := breakEnv(t, 10, "")
	a := open(t, e)
	db := e.RawDB(t)
	if _, err := db.Exec(`DROP TABLE recommend`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	got := breakTick(t, a, e, 9, 52, 0)
	wantBreak(t, "09:52", got, "52 min without a break. Stand up for 10 min.")
	if got[0].Kind != focus.Notify || got[0].RecID != 0 {
		t.Fatalf("event = %+v, want a plain Notify without a row", got[0])
	}
	if a.RoutineErr() == nil {
		t.Error("RoutineErr not set")
	}
}

const breakOverNote = "10 min break done. Back to it."

func TestBreakOverOnce(t *testing.T) {
	e := breakEnv(t, 10, "")
	a := open(t, e)
	ev := breakTick(t, a, e, 9, 52, 0)[0]
	slotClock(e, 9, 53, 0)
	if _, err := a.AnswerRecommend(ev.RecID, "break", "accept"); err != nil {
		t.Fatal(err)
	}
	wantList(t, "10:03:02", tickAt(t, a, e, 10, 3, 2), wantNote(focus.TopicBreakOver, "Break over", breakOverNote))
	wantList(t, "10:03:07", tickAt(t, a, e, 10, 3, 7))
	wantList(t, "restart 10:04", tickAt(t, open(t, e), e, 10, 4, 0))
	if got := stateVal(t, e, store.KeyFocusBreakOver); got != strconv.FormatInt(apptest.At(t, "10:03", 0), 10) {
		t.Fatalf("focus_break_over = %q", got)
	}
}

func TestBreakOverLate(t *testing.T) {
	e := breakEnv(t, 10, "")
	a := open(t, e)
	ev := breakTick(t, a, e, 9, 52, 0)[0]
	slotClock(e, 9, 53, 0)
	if _, err := a.AnswerRecommend(ev.RecID, "break", "accept"); err != nil {
		t.Fatal(err)
	}
	wantList(t, "10:09", tickAt(t, a, e, 10, 9, 0))
	wantList(t, "10:09:05", tickAt(t, a, e, 10, 9, 5))
}
