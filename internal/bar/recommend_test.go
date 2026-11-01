package bar

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/focus"
	"tock/internal/store"
)

// recApp is a Saturday 09:30 in the middle of the best focus hour, with a
// pending suggestion: its FocusTick note is returned.
func recApp(t *testing.T) (*apptest.Env, *app.App, focus.Event) {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.FocusHistory(21)...)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices+apptest.AllHours)
	e.Clock = time.Date(2026, 10, 10, 9, 30, 0, 0, apptest.Zone)
	a := openApp(t, e)
	evs, err := a.FocusTick(false)
	if err != nil || len(evs) != 1 || evs[0].Topic != focus.TopicRecommend {
		t.Fatalf("FocusTick = %+v, %v, want one recommend note", evs, err)
	}
	return e, a, evs[0]
}

func TestButtonsRecommend(t *testing.T) {
	n := store.Notice{ID: 5, Topic: focus.TopicRecommend}
	want := []Button{{"rec:accept", "Start focus (25 min)"}, {"rec:dismiss", "Not now"}}
	if got := Buttons(n, app.NoticeData{Rec: 3, Minutes: 25}); !reflect.DeepEqual(got, want) {
		t.Fatalf("buttons = %+v", got)
	}
	if got := Buttons(n, app.NoticeData{Minutes: 25}); got != nil {
		t.Fatalf("Rec 0 buttons = %+v, want nil", got)
	}
	if got := Category(n, app.NoticeData{}); got != "" {
		t.Fatalf("Rec 0 category = %q, want empty", got)
	}
	if got := Category(n, app.NoticeData{Rec: 3, Minutes: 25}); got != "recommend-5" {
		t.Fatalf("category = %q", got)
	}
	if RecAcceptTitle("focus", 45) != "Start focus (45 min)" || RecAcceptTitle("break", 10) != "Take a break (10 min)" {
		t.Fatal("RecAcceptTitle")
	}
}

func TestRespondRecommend(t *testing.T) {
	_, a, ev := recApp(t)
	id := enqueue(t, a, ev)
	if msg, err := Respond(a, id, ActRecAccept); err != nil || msg != "started 25 min session" {
		t.Fatalf("accept = %q, %v", msg, err)
	}
	if running, err := a.RunningSessionID(); err != nil || running == 0 {
		t.Fatalf("running session = %d, %v", running, err)
	}
	if msg, err := Respond(a, id, ActRecAccept); err != nil || msg != "that suggestion is no longer open" {
		t.Fatalf("second accept = %q, %v", msg, err)
	}

	_, a2, ev2 := recApp(t)
	id2 := enqueue(t, a2, ev2)
	if msg, err := Respond(a2, id2, ActRecDismiss); err != nil || msg != "suggestion dismissed" {
		t.Fatalf("dismiss = %q, %v", msg, err)
	}
	if running, _ := a2.RunningSessionID(); running != 0 {
		t.Fatal("dismiss started a session")
	}
}

func TestRespondRecommendSessionRunning(t *testing.T) {
	_, a, ev := recApp(t)
	id := enqueue(t, a, ev)
	if _, err := a.StartSession(10); err != nil {
		t.Fatal(err)
	}
	if msg, err := Respond(a, id, ActRecAccept); err != nil || msg != "a session is already running, nothing started" {
		t.Fatalf("accept = %q, %v", msg, err)
	}
	// The suggestion stays open to dismiss.
	if msg, err := Respond(a, id, ActRecDismiss); err != nil || msg != "suggestion dismissed" {
		t.Fatalf("dismiss = %q, %v", msg, err)
	}
}

// TestRespondRecommendSpoof: notice data is writable by any process, so the
// session length and the kind come from the recommend row.
func TestRespondRecommendSpoof(t *testing.T) {
	e, a, ev := recApp(t)
	db := e.RawDB(t)
	defer db.Close()
	spoof := func(rec int64) int64 {
		t.Helper()
		res, err := db.Exec(`INSERT INTO notice(at_ms, topic, title, body, data) VALUES(?, 'recommend', 'x', 'y', ?)`,
			e.Clock.UnixMilli(), `{"rec": `+strconv.FormatInt(rec, 10)+`, "minutes": 240}`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	if msg, err := Respond(a, spoof(ev.RecID), ActRecAccept); err != nil || msg != "started 25 min session" {
		t.Fatalf("spoofed minutes: %q, %v, want the row's 25 min", msg, err)
	}
	if _, err := a.StopSession(); err != nil {
		t.Fatal(err)
	}
	r, err := db.Exec(`INSERT INTO recommend(kind, at_ms, weekday, hour, length_ms) VALUES('focus', ?, 6, 9, 1500000)`,
		e.Clock.UnixMilli()-31*60_000)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := r.LastInsertId()
	if msg, err := Respond(a, spoof(old), ActRecAccept); err != nil || msg != "that suggestion is no longer open" {
		t.Fatalf("old row: %q, %v", msg, err)
	}
	if running, _ := a.RunningSessionID(); running != 0 {
		t.Fatal("an old row started a session")
	}
}
