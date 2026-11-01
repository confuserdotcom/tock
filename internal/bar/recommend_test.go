package bar

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/focus"
	"tock/internal/recommend"
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

// recIDs are the accept and dismiss menu ids of row id sent at atMS.
func recIDs(id, atMS int64) (accept, dismiss string) {
	k := strconv.FormatInt(id, 10) + ":" + strconv.FormatInt(atMS, 10)
	return "rec:accept:" + k, "rec:dismiss:" + k
}

func TestSnapshotRec(t *testing.T) {
	e, a, ev := recApp(t)
	now := e.Clock.UnixMilli()
	s, err := Load(a, e.Clock.Add(5*time.Minute), app.NotifyGranted)
	if err != nil {
		t.Fatal(err)
	}
	if s.Rec == nil || s.Rec.ID != ev.RecID || s.RecErr != nil {
		t.Fatalf("Rec = %+v, %v, want row %d", s.Rec, s.RecErr, ev.RecID)
	}
	acc, dis := recIDs(ev.RecID, now)
	items := s.Items(now + 5*60_000)
	got := titles(items)
	for _, want := range []string{"Suggested: focus now (25 min)", "Start focus (25 min) [" + acc + "]", "Not now [" + dis + "]"} {
		if !strings.Contains(got, want) {
			t.Errorf("menu lacks %q:\n%s", want, got)
		}
	}
	// The rows sit after the start items and before the trailing separator
	// block (settings / quit).
	idx := func(sub string) int {
		for i, it := range items {
			if strings.Contains(it.Title+it.ID, sub) {
				return i
			}
		}
		return -1
	}
	if !(idx("Start 25 min focus") < idx("Suggested:") && idx("Suggested:") < idx("Start focus (25") && idx("Start focus (25") < idx("Not now") && idx("Not now") < idx("Quit")) {
		t.Errorf("order wrong:\n%s", got)
	}

	// The same snapshot drawn when the row is 30 min old: expired.
	if got := titles(s.Items(now + 30*60_000)); strings.Contains(got, "Suggested") || strings.Contains(got, "Not now") {
		t.Errorf("expired row still on the menu:\n%s", got)
	}

	// A running session hides them.
	if _, err := a.StartSession(10); err != nil {
		t.Fatal(err)
	}
	s2, err := Load(a, e.Clock.Add(time.Minute), app.NotifyGranted)
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(s2.Items(now + 60_000)); strings.Contains(got, "Suggested") || strings.Contains(got, "Not now") {
		t.Errorf("rows shown with a session running:\n%s", got)
	}
}

func TestSnapshotRecBreak(t *testing.T) {
	s := Snapshot{OK: true, Presets: []int{25}, TrackdUp: true, Rec: &recommend.Rec{ID: 4, Kind: recommend.KindBreak, AtMS: 1_000, LengthMS: 10 * 60_000}}
	got := titles(s.Items(2_000))
	for _, want := range []string{"Suggested: break (10 min)", "Take a break (10 min) [rec:accept:4:1000]", "Not now [rec:dismiss:4:1000]"} {
		if !strings.Contains(got, want) {
			t.Errorf("menu lacks %q:\n%s", want, got)
		}
	}
}

func TestMenuRec(t *testing.T) {
	e, a, ev := recApp(t)
	now := e.Clock.UnixMilli()
	acc, _ := recIDs(ev.RecID, now)
	if msg, err := Menu(a, acc); err != nil || msg != "started 25 min session" {
		t.Fatalf("accept = %q, %v", msg, err)
	}
	if running, _ := a.RunningSessionID(); running == 0 {
		t.Fatal("no session running")
	}
	if msg, err := Menu(a, acc); err != nil || msg != "that suggestion is no longer open" {
		t.Fatalf("second accept = %q, %v", msg, err)
	}

	_, a2, ev2 := recApp(t)
	_, dis := recIDs(ev2.RecID, now)
	if msg, err := Menu(a2, dis); err != nil || msg != "suggestion dismissed" {
		t.Fatalf("dismiss = %q, %v", msg, err)
	}
	if running, _ := a2.RunningSessionID(); running != 0 {
		t.Fatal("dismiss started a session")
	}
	for _, bad := range []string{"rec:accept:abc", "rec:accept:0:5", "rec:accept:3", "rec:accept:3:0", "rec:dismiss:-1:5", "rec:dismiss:3:x", "rec:accept:"} {
		if _, err := Menu(a2, bad); !errors.Is(err, ErrUnknown) {
			t.Errorf("Menu(%q) err = %v, want ErrUnknown", bad, err)
		}
	}
}

// TestMenuRecStale: a click on a menu drawn before the row was answered, or
// before another row took its id, answers nothing.
func TestMenuRecStale(t *testing.T) {
	e, a, ev := recApp(t)
	now := e.Clock.UnixMilli()
	acc, _ := recIDs(ev.RecID, now)
	// The row goes away and a newer one takes the id.
	if err := a.ReleaseRecommend(ev); err != nil {
		t.Fatal(err)
	}
	e.Clock = e.Clock.Add(5 * time.Minute)
	a2 := openApp(t, e)
	s := openRaw(t, e)
	defer s.Close()
	id, err := s.AddRecommend(store.Recommend{Kind: "focus", AtMS: e.Clock.UnixMilli(), LengthMS: 10 * 60_000})
	if err != nil {
		t.Fatal(err)
	}
	if id != ev.RecID {
		t.Skipf("id %d not reused (got %d)", ev.RecID, id)
	}
	if msg, err := Menu(a2, acc); err != nil || msg != "that suggestion is no longer open" {
		t.Fatalf("stale accept = %q, %v", msg, err)
	}
	if running, _ := a2.RunningSessionID(); running != 0 {
		t.Fatal("a stale click started a session")
	}
}

func openRaw(t *testing.T, e *apptest.Env) *store.Store {
	t.Helper()
	s, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// breakApp is a Saturday 09:52 after 10 days of 52 min runs and a run since
// 09:00: the break reminder is pending and its notice is returned.
func breakApp(t *testing.T) (*apptest.Env, *app.App, focus.Event) {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	for i := 1; i <= 10; i++ {
		e.Seed(t, apptest.Span{ID: int64(2000 + i), Start: "08:00", End: "08:52", Bundle: "term", App: "WezTerm", Title: "tock │ h", Day: -i})
	}
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices)
	e.Clock = time.Date(2026, 10, 10, 9, 52, 0, 0, apptest.Zone)
	a := openApp(t, e)
	evs, err := a.FocusTick(false)
	if err != nil || len(evs) != 1 || evs[0].Topic != focus.TopicBreak || evs[0].RecID == 0 {
		t.Fatalf("FocusTick = %+v, %v, want one break note with a row", evs, err)
	}
	e.Clock = time.Date(2026, 10, 10, 9, 53, 0, 0, apptest.Zone)
	return e, a, evs[0]
}

func TestButtonsBreakRec(t *testing.T) {
	n := store.Notice{ID: 5, Topic: focus.TopicBreak}
	want := []Button{{"rec:accept", "Take a break (10 min)"}, {"rec:dismiss", "Not now"}, {"snooze", "Snooze 30 min"}}
	if got := Buttons(n, app.NoticeData{Rec: 7, Minutes: 10}); !reflect.DeepEqual(got, want) {
		t.Fatalf("buttons = %+v", got)
	}
	snooze := []Button{{"snooze", "Snooze 30 min"}}
	for _, d := range []app.NoticeData{{Minutes: 50}, {Rec: 7}, {}} {
		if got := Buttons(n, d); !reflect.DeepEqual(got, snooze) {
			t.Errorf("Buttons(%+v) = %+v, want the Snooze-only list", d, got)
		}
	}
}

func TestRespondBreak(t *testing.T) {
	_, a, ev := breakApp(t)
	id := enqueue(t, a, ev)
	if msg, err := Respond(a, id, ActRecAccept); err != nil || msg != "break until 10:03" {
		t.Fatalf("accept = %q, %v", msg, err)
	}
	if msg, err := Respond(a, id, ActRecAccept); err != nil || msg != "that suggestion is no longer open" {
		t.Fatalf("second accept = %q, %v", msg, err)
	}

	e, a2, ev2 := breakApp(t)
	id2 := enqueue(t, a2, ev2)
	if msg, err := Respond(a2, id2, ActRecDismiss); err != nil || msg != "suggestion dismissed" {
		t.Fatalf("dismiss = %q, %v", msg, err)
	}
	if until, _ := a2.BreakUntil(e.Clock); until != 0 {
		t.Fatal("dismiss started a break")
	}

	// Snooze still holds the reminders back for 30 min.
	_, a3, ev3 := breakApp(t)
	id3 := enqueue(t, a3, ev3)
	if msg, err := Respond(a3, id3, ActSnooze); err != nil || msg != "break reminders snoozed for 30 min" {
		t.Fatalf("snooze = %q, %v", msg, err)
	}
}

// A notice of one topic cannot answer a row of the other kind.
func TestRespondBreakKindMismatch(t *testing.T) {
	e, a, ev := breakApp(t)
	db := e.RawDB(t)
	defer db.Close()
	notice := func(topic string, rec int64) int64 {
		t.Helper()
		res, err := db.Exec(`INSERT INTO notice(at_ms, topic, title, body, data) VALUES(?, ?, 'x', 'y', ?)`,
			e.Clock.UnixMilli(), topic, `{"rec": `+strconv.FormatInt(rec, 10)+`, "minutes": 10}`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	// A recommend notice naming the break row.
	if msg, err := Respond(a, notice("recommend", ev.RecID), ActRecAccept); err != nil || msg != "that suggestion is no longer open" {
		t.Fatalf("recommend notice on a break row: %q, %v", msg, err)
	}
	// A break notice naming a focus row.
	r, err := db.Exec(`INSERT INTO recommend(kind, at_ms, weekday, hour, length_ms) VALUES('focus', ?, 6, 9, 1500000)`, e.Clock.UnixMilli()-60_000)
	if err != nil {
		t.Fatal(err)
	}
	focusRow, _ := r.LastInsertId()
	if msg, err := Respond(a, notice("break", focusRow), ActRecAccept); err != nil || msg != "that suggestion is no longer open" {
		t.Fatalf("break notice on a focus row: %q, %v", msg, err)
	}
	if running, _ := a.RunningSessionID(); running != 0 {
		t.Fatal("a mismatched answer started a session")
	}
	if until, _ := a.BreakUntil(e.Clock); until != 0 {
		t.Fatal("a mismatched answer started a break")
	}
	var reply string
	if err := db.QueryRow(`SELECT reply FROM recommend WHERE id=?`, ev.RecID).Scan(&reply); err != nil || reply != "" {
		t.Fatalf("break row reply = %q, %v, want unchanged", reply, err)
	}
	if err := db.QueryRow(`SELECT reply FROM recommend WHERE id=?`, focusRow).Scan(&reply); err != nil || reply != "" {
		t.Fatalf("focus row reply = %q, %v, want unchanged", reply, err)
	}
}
