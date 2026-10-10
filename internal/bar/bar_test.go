package bar

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/store"
)

func openApp(t *testing.T, e *apptest.Env) *app.App {
	t.Helper()
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func seeded(t *testing.T) (*apptest.Env, *app.App) {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Heartbeat(t, e.Clock.UnixMilli())
	return e, openApp(t, e)
}

func titles(items []Item) string {
	var b []string
	for _, it := range items {
		switch {
		case it.Separator:
			b = append(b, "--")
		case it.ID == "":
			b = append(b, it.Title)
		default:
			b = append(b, it.Title+" ["+it.ID+"]")
		}
	}
	return strings.Join(b, "\n")
}

func TestSnapshotDay(t *testing.T) {
	e, a := seeded(t)
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil {
		t.Fatal(err)
	}
	now := e.Clock.UnixMilli()
	if got := s.Title(now); got != "◷ 2h54" {
		t.Fatalf("title = %q", got)
	}
	want := `Today · 2h54 tracked
Focus 2h14 · score 74
28m unlabelled · open tock to label
--
Start 25 min focus [start:25]
Start 50 min focus [start:50]
Start 90 min focus [start:90]
--
Quit tock menu bar [quit]`
	if got := titles(s.Items(now)); got != want {
		t.Fatalf("menu:\n%s\nwant:\n%s", got, want)
	}

	s.TrackdUp, s.Access = false, app.NotifyDenied
	if got := s.Title(now); got != "◷ 2h54 !" {
		t.Fatalf("down title = %q", got)
	}
	got := titles(s.Items(now))
	if !strings.Contains(got, "trackd is not running\n--") || !strings.Contains(got, "Notifications are off… [settings]") {
		t.Fatalf("down and denied menu:\n%s", got)
	}
}

func TestSnapshotSession(t *testing.T) {
	e, a := seeded(t)
	if _, err := Menu(a, "start:25"); err != nil {
		t.Fatal(err)
	}
	e.Clock = e.Clock.Add(6*time.Minute + 18*time.Second)
	s, err := Load(a, e.Clock, app.NotifyGranted)
	if err != nil {
		t.Fatal(err)
	}
	now := e.Clock.UnixMilli()
	if got := s.Title(now); got != "◉ 18:42" {
		t.Fatalf("title = %q", got)
	}
	got := titles(s.Items(now))
	if !strings.Contains(got, "Session · 18:42 left · 0 interruptions") || !strings.Contains(got, "Stop session [stop]") ||
		strings.Contains(got, "start:") {
		t.Fatalf("session menu:\n%s", got)
	}
	if msg, err := Menu(a, IDStop); err != nil || msg != "stopped session after 6m" {
		t.Fatalf("stop = %q, %v", msg, err)
	}
	if _, err := Menu(a, IDStop); !errors.Is(err, store.ErrNoSession) {
		t.Fatalf("second stop = %v", err)
	}
	if _, err := Menu(a, "bogus"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("unknown id = %v", err)
	}
}

func TestSnapshotNotSetUp(t *testing.T) {
	var s Snapshot
	if s.Title(0) != "◷ tock" || !strings.HasPrefix(titles(s.Items(0)), "tock is not set up") {
		t.Fatalf("not set up: %q / %q", s.Title(0), titles(s.Items(0)))
	}
}

func TestPresetsCapped(t *testing.T) {
	s := Snapshot{OK: true, TrackdUp: true, Presets: []int{10, 20, 30, 40}}
	if got := titles(s.Items(0)); strings.Contains(got, "40") || !strings.Contains(got, "start:30") {
		t.Fatalf("menu:\n%s", got)
	}
}

func TestButtons(t *testing.T) {
	away := store.Notice{ID: 7, Topic: focus.TopicAway}
	d := app.NoticeData{Labels: []classify.Want{{Category: "Meetings"}, {Category: "Code", Project: "tock"}}}
	want := []Button{{"away:0", "Meetings"}, {"away:1", "Code / tock"}, {"idle", "Leave idle"}}
	if got := Buttons(away, d); !reflect.DeepEqual(got, want) {
		t.Fatalf("away buttons = %+v", got)
	}
	if Category(away) != "away-7" || Category(store.Notice{Topic: focus.TopicBreak}) != "break" {
		t.Fatal("categories")
	}
	for topic, want := range map[string]string{
		focus.TopicFocus: "Stop session", focus.TopicDone: "Start again (50 min)", focus.TopicBreak: "Snooze 30 min",
	} {
		b := Buttons(store.Notice{Topic: topic}, app.NoticeData{Minutes: 50})
		if len(b) != 1 || b[0].Title != want {
			t.Fatalf("%s buttons = %+v", topic, b)
		}
	}
	if b := Buttons(store.Notice{Topic: focus.TopicRecap}, app.NoticeData{}); b != nil {
		t.Fatalf("recap buttons = %+v", b)
	}
}

func enqueue(t *testing.T, a *app.App, ev focus.Event) int64 {
	t.Helper()
	if err := a.Enqueue(ev); err != nil {
		t.Fatal(err)
	}
	ns, err := a.TakeNotices()
	if err != nil || len(ns) != 1 {
		t.Fatalf("take = %+v, %v", ns, err)
	}
	return ns[0].ID
}

func TestRespond(t *testing.T) {
	e, a := seeded(t)
	done := enqueue(t, a, focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "b", Minutes: 50})
	if msg, err := Respond(a, done, ActAgain); err != nil || msg != "started 50 min session" {
		t.Fatalf("again = %q, %v", msg, err)
	}
	if _, err := Respond(a, done, ActAgain); !errors.Is(err, store.ErrSessionRunning) {
		t.Fatalf("again while running = %v", err)
	}
	stop := enqueue(t, a, focus.Event{Topic: focus.TopicFocus, Title: focus.TitleFocus, Body: "b"})
	if msg, err := Respond(a, stop, ActStop); err != nil || !strings.HasPrefix(msg, "stopped session") {
		t.Fatalf("stop = %q, %v", msg, err)
	}
	brk := enqueue(t, a, focus.Event{Topic: focus.TopicBreak, Title: focus.TitleBreak, Body: "b"})
	if msg, err := Respond(a, brk, ActSnooze); err != nil || msg != "break reminders snoozed for 30 min" {
		t.Fatalf("snooze = %q, %v", msg, err)
	}
	// A body click, or an action the notice does not offer, does nothing.
	for _, act := range []string{"com.apple.UNNotificationDefaultActionIdentifier", ActStop, "away:9"} {
		if msg, err := Respond(a, brk, act); err != nil || msg != "" {
			t.Fatalf("%s = %q, %v", act, msg, err)
		}
	}
	if _, err := Respond(a, 9999, ActStop); !errors.Is(err, store.ErrNoNotice) {
		t.Fatalf("missing notice = %v", err)
	}

	// Away: label with the first quick label, then leave another idle.
	if err := a.Answer(classify.Interval{From: apptest.At(t, "08:00", 0), To: apptest.At(t, "08:30", 0)},
		classify.Want{Category: "Communication"}); err != nil {
		t.Fatal(err)
	}
	day, err := a.Day(e.Clock)
	if err != nil {
		t.Fatal(err)
	}
	iv := day.Away
	if len(iv) == 0 {
		t.Fatal("seeded day has no pending absence")
	}
	away := enqueue(t, a, focus.Event{Topic: focus.TopicAway, Title: focus.TitleAway, Body: "b", Away: iv[0]})
	if msg, err := Respond(a, away, "away:0"); err != nil || msg != "labelled away time" {
		t.Fatalf("away label = %q, %v", msg, err)
	}
	d, err := a.Day(e.Clock)
	if err != nil || len(d.Away) != len(iv)-1 {
		t.Fatalf("absence still pending: %+v, %v", d.Away, err)
	}
}
