package main

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/store"
)

type recNotifier struct{ got []string }

func (r *recNotifier) Notify(title, body string) error {
	r.got = append(r.got, title+"|"+body)
	return nil
}

// distractedEnv: 5 min of Code from 14:50, then YouTube (open) from 14:55,
// and a 25 min session started at 14:50. The clock is 15:00.
func distractedEnv(t *testing.T, rules string) *apptest.Env {
	t.Helper()
	e := apptest.New(t, rules)
	e.Seed(t,
		apptest.Span{ID: 1, Start: "14:50", End: "14:55", Bundle: "term", App: "WezTerm", Title: "tock │ a"},
		apptest.Span{ID: 2, Start: "14:55", Bundle: "arc", App: "Arc", Title: "Video", URL: "www.youtube.com/watch"},
	)
	e.WriteConfig(t, apptest.NoRoutines)
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.StartSession(apptest.At(t, "14:50", 0), apptest.At(t, "15:15", 0)); err != nil {
		t.Fatal(err)
	}
	return e
}

func testLoop(t *testing.T, e *apptest.Env, n Notifier, camera func() bool) (*focusLoop, *bytes.Buffer) {
	t.Helper()
	var logb bytes.Buffer
	f := &focusLoop{
		open: func() (*app.App, error) {
			return app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
		},
		camera: camera, notify: n, log: slog.New(slog.NewTextHandler(&logb, nil)),
	}
	t.Cleanup(f.close)
	return f, &logb
}

func TestFocusLoopAlertsOnce(t *testing.T) {
	e := distractedEnv(t, apptest.Rules)
	n := &recNotifier{}
	f, logb := testLoop(t, e, n, nil)
	f.tick()
	f.tick()
	want := []string{"tock · focus|5 min distracted in your 25 min session, 15 min left."}
	if !reflect.DeepEqual(n.got, want) {
		t.Fatalf("notifications = %q, want %q", n.got, want)
	}
	g, _ := testLoop(t, e, n, nil) // trackd restarted
	g.tick()
	if len(n.got) != 1 {
		t.Fatalf("restart repeated the alert: %q", n.got)
	}
	for _, s := range append(n.got, logb.String()) {
		if strings.Contains(s, "Video") || strings.Contains(s, "youtube") {
			t.Fatalf("title or URL leaked: %q", s)
		}
	}
}

func TestFocusLoopSendsStartOfDay(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ a"})
	e.Clock = time.Date(2026, 10, 10, 9, 30, 0, 0, apptest.Zone)
	n := &recNotifier{}
	f, _ := testLoop(t, e, n, nil)
	f.tick()
	f.tick()
	g, _ := testLoop(t, e, n, nil) // trackd restarted
	g.tick()
	want := []string{"Start of day|Yesterday: no activity."}
	if !reflect.DeepEqual(n.got, want) {
		t.Fatalf("notifications = %q, want %q", n.got, want)
	}
}

// slotEnv: a plan file with Review at 14:00 and Break at 14:50, a span open
// since 13:50 and the clock at 14:00:30.
func slotEnv(t *testing.T) *apptest.Env {
	t.Helper()
	e := apptest.New(t, apptest.Rules)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	if err := os.WriteFile(todo, []byte("Review @14h00 due:2026-10-10\nBreak @14h50 due:2026-10-10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.WriteConfig(t, apptest.NoRoutines+"[plan]\nfile = '"+todo+"'\n")
	e.Seed(t, apptest.Span{ID: 1, Start: "13:50", Bundle: "term", App: "WezTerm", Title: "x"})
	e.Clock = time.Date(2026, 10, 10, 14, 0, 30, 0, apptest.Zone)
	return e
}

func TestFocusLoopSendsSlotStartOnce(t *testing.T) {
	e := slotEnv(t)
	n := &recNotifier{}
	f, _ := testLoop(t, e, n, nil)
	f.tick()
	f.tick()
	g, _ := testLoop(t, e, n, nil) // trackd restarted
	g.tick()
	want := []string{"Starting now|14:00–14:50 Review"}
	if !reflect.DeepEqual(n.got, want) {
		t.Fatalf("notifications = %q, want %q", n.got, want)
	}
}

func TestFocusLoopCameraMutes(t *testing.T) {
	e := distractedEnv(t, apptest.Rules)
	n := &recNotifier{}
	f, _ := testLoop(t, e, n, func() bool { return true })
	f.tick()
	if len(n.got) != 0 {
		t.Fatalf("camera on: %q", n.got)
	}
}

func TestFocusLoopMicMutesAndRecords(t *testing.T) {
	e := distractedEnv(t, apptest.Rules)
	n := &recNotifier{}
	f, _ := testLoop(t, e, n, nil)
	f.mic = func() bool { return true }
	f.tick()
	e.Clock = e.Clock.Add(5 * time.Second)
	f.tick()
	if len(n.got) != 0 {
		t.Fatalf("mic on: %q", n.got)
	}
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	calls, err := st.Calls(0, e.Clock.UnixMilli()+1)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].EndMS-calls[0].StartMS != 5000 {
		t.Fatalf("calls = %v, want one 5 s call", calls)
	}
}

func TestFocusLoopNotifyOff(t *testing.T) {
	e := distractedEnv(t, apptest.Rules)
	e.WriteConfig(t, "[focus]\nnotify = false\n"+apptest.NoRoutines)
	n := &recNotifier{}
	f, _ := testLoop(t, e, n, nil)
	f.tick()
	if len(n.got) != 0 {
		t.Fatalf("notify off: %q", n.got)
	}
	st, err := store.Open(e.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if o, err := st.OpenSession(); err != nil || o == nil || o.Interruptions != 1 {
		t.Fatalf("interruption not recorded: %+v, %v", o, err)
	}
}

func TestFocusLoopWaitsForRules(t *testing.T) {
	e := distractedEnv(t, "")
	n := &recNotifier{}
	f, logb := testLoop(t, e, n, nil)
	f.tick()
	f.tick()
	if c := strings.Count(logb.String(), "focus engine waiting"); c != 1 {
		t.Fatalf("waiting logged %d times:\n%s", c, logb)
	}
	e.WriteRules(t, apptest.Rules)
	f.tick()
	if !strings.Contains(logb.String(), "focus engine started") || len(n.got) != 1 {
		t.Fatalf("after rules: %q\n%s", n.got, logb)
	}
}

func TestNotifyArgsKeepTextOutOfTheScript(t *testing.T) {
	title, body := `x" & (do shell script "id") & "`, "end run"
	want := []string{"-e", "on run argv",
		"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
		"-e", "end run", title, body}
	if got := notifyArgs(title, body); !reflect.DeepEqual(got, want) {
		t.Fatalf("notifyArgs = %q", got)
	}
}

func TestFocusLoopHandsNotesToTheBar(t *testing.T) {
	e := distractedEnv(t, apptest.Rules)
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.BarBeat(app.NotifyGranted); err != nil {
		t.Fatal(err)
	}
	n := &recNotifier{}
	f, logb := testLoop(t, e, n, nil)
	f.tick()
	if len(n.got) != 0 {
		t.Fatalf("the notifier ran while the bar is ready: %q", n.got)
	}
	ns, err := a.TakeNotices()
	a.Close()
	if err != nil || len(ns) != 1 || ns[0].Topic != "focus" || ns[0].Body != "5 min distracted in your 25 min session, 15 min left." {
		t.Fatalf("queued = %+v, %v", ns, err)
	}
	if !strings.Contains(logb.String(), "focus notification queued") || strings.Contains(logb.String(), "distracted") {
		t.Fatalf("log must name the title only: %s", logb.String())
	}
}

func TestFocusLoopFallsBackWhenBarDenied(t *testing.T) {
	e := distractedEnv(t, apptest.Rules)
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	if err := a.BarBeat(app.NotifyDenied); err != nil {
		t.Fatal(err)
	}
	a.Close()
	n := &recNotifier{}
	f, _ := testLoop(t, e, n, nil)
	f.tick()
	if len(n.got) != 1 {
		t.Fatalf("denied bar: notifier got %q", n.got)
	}
}

func TestFocusLoopRoutinesNotifyOff(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ a"})
	e.WriteConfig(t, "[focus]\nnotify = false\n[hours]\ndays = [\"mon\",\"tue\",\"wed\",\"thu\",\"fri\",\"sat\",\"sun\"]\nstart = \"09:00\"\nend = \"09:10\"\n")
	e.Clock = time.Date(2026, 10, 10, 9, 45, 0, 0, apptest.Zone)
	n := &recNotifier{}
	f, _ := testLoop(t, e, n, nil)
	f.tick()
	f.tick()
	if len(n.got) != 0 {
		t.Fatalf("notify off: %q", n.got)
	}
}

// TestFocusLoopWarnRoutineOnce: a routine notice that cannot be built is
// logged once while it lasts and again after it recovered.
func TestFocusLoopWarnRoutineOnce(t *testing.T) {
	var logb bytes.Buffer
	f := &focusLoop{log: slog.New(slog.NewTextHandler(&logb, nil))}
	bad := errors.New("app: start-of-day notice: boom")
	f.warnRoutine(bad)
	f.warnRoutine(bad)
	if n := strings.Count(logb.String(), "routine notice skipped"); n != 1 {
		t.Fatalf("logged %d times, want 1: %q", n, logb.String())
	}
	f.warnRoutine(nil)
	f.warnRoutine(bad)
	if n := strings.Count(logb.String(), "routine notice skipped"); n != 2 {
		t.Fatalf("logged %d times after recovery, want 2", n)
	}
}

// flakyNotifier fails its first fails calls.
type flakyNotifier struct {
	fails int
	got   []string
}

func (f *flakyNotifier) Notify(title, body string) error {
	if f.fails > 0 {
		f.fails--
		return errors.New("osascript failed")
	}
	f.got = append(f.got, title+"|"+body)
	return nil
}

// A slot notice that could not be delivered is not lost: it was marked as
// sent, so the loop releases the mark and the next tick of the window sends it.
func TestFocusLoopRetriesSlotNoticeAfterFailure(t *testing.T) {
	e := slotEnv(t)
	n := &flakyNotifier{fails: 1}
	f, _ := testLoop(t, e, n, nil)
	f.tick()
	if len(n.got) != 0 {
		t.Fatalf("first tick delivered %q, want the failure", n.got)
	}
	f.tick()
	f.tick()
	want := []string{"Starting now|14:00–14:50 Review"}
	if !reflect.DeepEqual(n.got, want) {
		t.Fatalf("notifications = %q, want %q", n.got, want)
	}
}
