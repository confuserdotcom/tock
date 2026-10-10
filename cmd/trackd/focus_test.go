package main

import (
	"bytes"
	"log/slog"
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
	e.WriteConfig(t, "[focus]\nnotify = false\n")
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
