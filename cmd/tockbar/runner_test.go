package main

import (
	"bytes"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/focus"
)

type fakeUI struct {
	titles, menus []string
	posts         []string
	auth          []bool
	urls          []string
	quit          bool
}

func (f *fakeUI) SetTitle(t string) { f.titles = append(f.titles, t) }
func (f *fakeUI) SetMenu(s string)  { f.menus = append(f.menus, s) }
func (f *fakeUI) Post(id int64, title, body, cat, cats string) {
	f.posts = append(f.posts, title+"|"+body+"|"+cat+"|"+cats)
}
func (f *fakeUI) CheckAuth(prompt bool) { f.auth = append(f.auth, prompt) }
func (f *fakeUI) OpenURL(u string)      { f.urls = append(f.urls, u) }
func (f *fakeUI) Quit()                 { f.quit = true }

func testRunner(t *testing.T, e *apptest.Env) (*runner, *fakeUI, *bytes.Buffer) {
	t.Helper()
	var logb bytes.Buffer
	u := &fakeUI{}
	r := &runner{
		open: func() (*app.App, error) {
			return app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
		},
		now: e.NowFunc(), log: slog.New(slog.NewTextHandler(&logb, nil)), prompt: true, ui: u,
	}
	t.Cleanup(r.close)
	return r, u, &logb
}

func seededEnv(t *testing.T) *apptest.Env {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.DaySpans()...)
	e.Heartbeat(t, e.Clock.UnixMilli())
	return e
}

func last(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}

func TestRunnerDrawsAndBeats(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	if last(u.titles) != "◷ 2h54" || !strings.Contains(last(u.menus), "start:25\tStart 25 min focus") {
		t.Fatalf("title %q menu %q", u.titles, last(u.menus))
	}
	if len(u.auth) != 1 || !u.auth[0] {
		t.Fatalf("first check must prompt: %v", u.auth)
	}
	r.handle(event{kind: evAuth, id: app.NotifyGranted})
	e.Clock = e.Clock.Add(beatEvery)
	r.tick()
	if len(u.auth) != 2 || u.auth[1] {
		t.Fatalf("later checks must not prompt: %v", u.auth)
	}
	if !r.a.BarReady() {
		t.Fatal("heartbeat with granted access must make the bar ready")
	}
	n := len(u.menus)
	r.tick()
	if len(u.menus) != n {
		t.Fatal("an unchanged menu must not be redrawn")
	}
}

func TestRunnerPostsAndResponds(t *testing.T) {
	e := seededEnv(t)
	r, u, logb := testRunner(t, e)
	r.handle(event{kind: evReady})
	if err := r.a.Enqueue(focus.Event{Topic: focus.TopicDone, Title: focus.TitleDone, Body: "25 min: done.", Minutes: 25}); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.posts) != 1 {
		t.Fatalf("posts = %q", u.posts)
	}
	f := strings.Split(u.posts[0], "|")
	if f[0] != focus.TitleDone || !strings.HasPrefix(f[2], "done-") || f[3] != f[2]+"\tagain\tStart again (25 min)" {
		t.Fatalf("post = %q", u.posts[0])
	}
	nid, err := strconv.ParseInt(strings.TrimPrefix(f[2], "done-"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	r.handle(event{kind: evResponse, notice: nid, id: "again"})
	if !strings.HasPrefix(last(u.titles), "◉ 25:00") {
		t.Fatalf("title after Start again = %q", last(u.titles))
	}
	r.handle(event{kind: evMenu, id: "stop"})
	if last(u.titles) != "◷ 2h54" {
		t.Fatalf("title after stop = %q", last(u.titles))
	}
	if !strings.Contains(logb.String(), "started 25 min session") || strings.Contains(logb.String(), "25 min: done.") {
		t.Fatalf("log must hold results, never bodies:\n%s", logb.String())
	}
	r.tick()
	if len(u.posts) != 1 {
		t.Fatal("a notice is shown once")
	}
}

func TestRunnerQuitAndSettings(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evAuth, id: app.NotifyDenied})
	r.tick()
	if !strings.Contains(last(u.menus), "settings\tNotifications are off…") {
		t.Fatalf("denied menu = %q", last(u.menus))
	}
	r.handle(event{kind: evMenu, id: "settings"})
	r.handle(event{kind: evMenu, id: "quit"})
	if len(u.urls) != 1 || !u.quit {
		t.Fatalf("urls %v quit %v", u.urls, u.quit)
	}
}

func TestRunnerWaitsForTock(t *testing.T) {
	e := apptest.New(t, apptest.Rules) // no database yet
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	if last(u.titles) != "◷ tock" || !strings.Contains(last(u.menus), "tock is not set up") {
		t.Fatalf("title %q menu %q", last(u.titles), last(u.menus))
	}
	e.Seed(t, apptest.DaySpans()...)
	e.Clock = e.Clock.Add(time.Second)
	r.tick()
	if r.a != nil {
		t.Fatal("reopen must wait reopenEvery")
	}
	e.Clock = e.Clock.Add(reopenEvery)
	r.tick()
	if r.a == nil || last(u.titles) == "◷ tock" {
		t.Fatalf("not reopened: %q", last(u.titles))
	}
}
