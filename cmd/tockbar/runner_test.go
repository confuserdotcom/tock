package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
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

func TestRunnerPostsSlotNotice(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	if err := os.WriteFile(todo, []byte("Review @14h00 due:2026-10-10\nBreak @14h50 due:2026-10-10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.WriteConfig(t, apptest.NoRoutines+"[plan]\nfile = '"+todo+"'\n")
	e.Seed(t, apptest.Span{ID: 1, Start: "13:50", Bundle: "term", App: "WezTerm", Title: "x"})
	e.Clock = time.Date(2026, 10, 10, 14, 0, 30, 0, apptest.Zone)
	e.Heartbeat(t, e.Clock.UnixMilli())
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	evs, err := r.a.FocusTick(false)
	if err != nil || len(evs) != 1 {
		t.Fatalf("FocusTick = %+v, %v, want the one slot notice", evs, err)
	}
	if err := r.a.Enqueue(evs[0]); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.posts) != 1 || u.posts[0] != "Starting now|14:00–14:50 Review||" {
		t.Fatalf("posts = %q, want one starting notice without a category", u.posts)
	}
}

func TestRunnerDrawsSubmenu(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	if _, err := r.a.StartSession(25); err != nil {
		t.Fatal(err)
	}
	e.Clock = e.Clock.Add(25 * time.Minute)
	x, err := r.a.StopSession()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.a.RateSession(x.ID, 4, nil); err != nil {
		t.Fatal(err)
	}
	r.dirty = true
	r.tick()
	id := strconv.FormatInt(x.ID, 10)
	menu := last(u.menus)
	var at int
	for _, want := range []string{
		"\n\tLast session · 25m · Q", "\n>\t15:00–15:25 · ", "\n>\tshare ", "\n>-\n",
		"\n>rate:5:" + id + "\tRate ★5\n", "\n>rate:4:" + id + "\tRate ★4\tc\n", "\n>rate:1:" + id + "\tRate ★1",
	} {
		i := strings.Index(menu[at:], want)
		if i < 0 {
			t.Fatalf("missing %q after offset %d in:\n%s", want, at, menu)
		}
		at += i + len(want) - 1
	}
	if !strings.Contains(menu, "start:25\tStart 25 min focus") {
		t.Fatalf("start item gone:\n%s", menu)
	}
	if strings.Contains(menu, "\n>>") {
		t.Fatalf("submenu nested wrongly:\n%s", menu)
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

const testMeetLink = "https://meet.google.com/abc-defg-hij"

func slotNotice(topic, link string) focus.Event {
	return focus.Event{Kind: focus.Notify, Topic: topic, Title: "Starting now", Body: "14:00–14:30 Standup",
		Slot: focus.Slot{Key: "event:1:x", Kind: focus.SlotEvent, JoinURL: link}}
}

func TestRunnerJoinOpensLink(t *testing.T) {
	e := seededEnv(t)
	r, u, logb := testRunner(t, e)
	r.handle(event{kind: evReady})
	if err := r.a.Enqueue(slotNotice(focus.TopicStarting, testMeetLink)); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.posts) != 1 {
		t.Fatalf("posts = %q", u.posts)
	}
	f := strings.Split(u.posts[0], "|")
	if !strings.HasPrefix(f[2], "starting-") || !strings.Contains(f[3], f[2]+"\tjoin\tJoin") {
		t.Fatalf("post = %q", u.posts[0])
	}
	if len(u.urls) != 0 {
		t.Fatalf("auto_open off, but opened %q", u.urls)
	}
	nid, err := strconv.ParseInt(strings.TrimPrefix(f[2], "starting-"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	r.handle(event{kind: evResponse, notice: nid, id: "join"})
	if len(u.urls) != 1 || u.urls[0] != testMeetLink {
		t.Fatalf("urls = %q", u.urls)
	}
	if strings.Contains(logb.String(), "meet.google") || strings.Contains(logb.String(), "abc-defg") {
		t.Fatalf("log holds the link:\n%s", logb.String())
	}
	// A body click is not a button: nothing opens.
	r.handle(event{kind: evResponse, notice: nid, id: "com.apple.UNNotificationDefaultActionIdentifier"})
	if len(u.urls) != 1 {
		t.Fatalf("urls after body click = %q", u.urls)
	}
}

func TestRunnerJoinIgnoresTamperedLink(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	if err := r.a.Enqueue(slotNotice(focus.TopicStarting, testMeetLink)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RawDB(t).Exec(`UPDATE notice SET data = '{"join":"javascript:alert(1)"}'`); err != nil {
		t.Fatal(err)
	}
	r.tick()
	r.handle(event{kind: evResponse, notice: 1, id: "join"})
	if len(u.urls) != 0 {
		t.Fatalf("opened %q", u.urls)
	}
}

func TestRunnerAutoOpen(t *testing.T) {
	setup := func(t *testing.T, cfg string) (*runner, *fakeUI) {
		e := seededEnv(t)
		e.WriteConfig(t, apptest.NoRoutines+cfg)
		r, u, _ := testRunner(t, e)
		r.handle(event{kind: evReady})
		return r, u
	}
	r, u := setup(t, "[calendar]\nauto_open = true\n")
	if err := r.a.Enqueue(slotNotice(focus.TopicStarting, testMeetLink)); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.urls) != 1 || u.urls[0] != testMeetLink {
		t.Fatalf("urls = %q, want the link once", u.urls)
	}
	r.tick()
	if len(u.urls) != 1 {
		t.Fatalf("opened again: %q", u.urls)
	}
	for name, ev := range map[string]focus.Event{
		"up next":      slotNotice(focus.TopicUpcoming, testMeetLink),
		"without link": slotNotice(focus.TopicStarting, ""),
		"bad link":     slotNotice(focus.TopicStarting, "https://evil.example/x"),
	} {
		r, u := setup(t, "[calendar]\nauto_open = true\n")
		if err := r.a.Enqueue(ev); err != nil {
			t.Fatal(err)
		}
		r.tick()
		if len(u.urls) != 0 {
			t.Errorf("%s: opened %q", name, u.urls)
		}
	}
	r, u = setup(t, "")
	if err := r.a.Enqueue(slotNotice(focus.TopicStarting, testMeetLink)); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.urls) != 0 || len(u.posts) != 1 {
		t.Fatalf("auto_open off: urls %q posts %q", u.urls, u.posts)
	}
}
