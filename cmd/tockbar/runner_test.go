package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
	"tock/internal/bar"
	"tock/internal/classify"
	"tock/internal/focus"
)

type fakeUI struct {
	titles, menus []string
	posts         []string
	auth          []bool
	urls          []string
	curtains      []string
	quit          bool
}

func (f *fakeUI) SetTitle(t string) { f.titles = append(f.titles, t) }
func (f *fakeUI) SetMenu(s string)  { f.menus = append(f.menus, s) }
func (f *fakeUI) Post(id int64, title, body, cat, cats string) {
	f.posts = append(f.posts, title+"|"+body+"|"+cat+"|"+cats)
}
func (f *fakeUI) Curtain(id int64, title, body, word string, fromMS, untilMS int64, buttons string) {
	f.curtains = append(f.curtains, fmt.Sprintf("%d|%s|%s|%s|%d|%d|%s", id, title, body, word, fromMS, untilMS, buttons))
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
		rules: e.Rules,
	}
	starts := 0
	r.dash = &dash{cmd: fakeWeb("serve", &starts)}
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
	r.handle(event{kind: evMenu, id: bar.IDNotifySettings})
	r.handle(event{kind: evMenu, id: "quit"})
	if len(u.urls) != 1 || !u.quit {
		t.Fatalf("urls %v quit %v", u.urls, u.quit)
	}
}

func TestRunnerWaitsForTock(t *testing.T) {
	e := apptest.New(t, apptest.Rules) // no database yet
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	if last(u.titles) != "◷ tock" || !strings.Contains(last(u.menus), "Waiting for trackd to start") {
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

func TestRunnerSetsUpRules(t *testing.T) {
	e := seededEnv(t)
	if err := os.Remove(e.Rules); err != nil {
		t.Fatal(err)
	}
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	if !strings.Contains(last(u.menus), "setup\tSet up tock") {
		t.Fatalf("menu = %q", last(u.menus))
	}
	r.handle(event{kind: evMenu, id: bar.IDSetup})
	if r.a == nil || last(u.titles) == "◷ tock" {
		t.Fatalf("not opened after setup: %q %q", last(u.titles), last(u.menus))
	}
	if b, err := os.ReadFile(e.Rules); err != nil || len(b) == 0 {
		t.Fatalf("rules not written: %v", err)
	}
	// A second click (stale menu) must not touch the rules now in place.
	if err := os.WriteFile(e.Rules, []byte(apptest.Rules), 0o600); err != nil {
		t.Fatal(err)
	}
	r.handle(event{kind: evMenu, id: bar.IDSetup})
	if b, _ := os.ReadFile(e.Rules); string(b) != apptest.Rules {
		t.Fatal("setup overwrote existing rules")
	}
}

func TestRunnerOpensDashboard(t *testing.T) {
	e := seededEnv(t)
	r, u, logb := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evMenu, id: bar.IDDashboard})
	r.handle(event{kind: evMenu, id: bar.IDInbox})
	r.handle(event{kind: evMenu, id: bar.IDAccess})
	if len(u.urls) != 3 {
		t.Fatalf("urls = %v\n%s", u.urls, logb)
	}
	if !strings.HasPrefix(u.urls[0], "http://127.0.0.1:") || !strings.Contains(u.urls[1], "/?v=inbox#") || u.urls[2] != bar.AccessibilityURL {
		t.Fatalf("urls = %v", u.urls)
	}
}

const testMeetLink = "https://meet.google.com/abc-defg-hij"

func slotNotice(topic, link string) focus.Event {
	return focus.Event{Kind: focus.Notify, Topic: topic, Title: "Starting now", Body: "14:00–14:30 Standup",
		Slot: focus.Slot{Key: "event:1:x", Kind: focus.SlotEvent, JoinURL: link, Mine: true}}
}

// unansweredNotice is a starting notice of an invite the owner has not
// accepted: it offers Join but never opens by itself.
func unansweredNotice(link string) focus.Event {
	ev := slotNotice(focus.TopicStarting, link)
	ev.Slot.Mine = false
	return ev
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
		"unanswered":   unansweredNotice(testMeetLink),
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

func TestRunnerMenuJoin(t *testing.T) {
	e := seededEnv(t)
	e.WriteConfig(t, apptest.NoRoutines)
	e.EventLink(t, "14:30", "15:00", "Standup", testMeetLink)
	e.Clock = time.Date(2026, 10, 10, 14, 25, 0, 0, apptest.Zone)
	r, u, logb := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.tick()
	menu := last(u.menus)
	var id string
	for _, line := range strings.Split(menu, "\n") {
		if strings.HasSuffix(line, "\tJoin Standup") {
			id = strings.TrimSuffix(line, "\tJoin Standup")
		}
	}
	if id == "" || !strings.HasPrefix(id, bar.IDJoin) {
		t.Fatalf("menu has no Join Standup line:\n%s", menu)
	}
	if !strings.Contains(menu, "\tNext: 14:30 Standup (in 5 min)") {
		t.Errorf("menu has no Next row:\n%s", menu)
	}
	r.handle(event{kind: evMenu, id: id})
	if len(u.urls) != 1 || u.urls[0] != testMeetLink {
		t.Fatalf("urls = %q, want the link once", u.urls)
	}
	if strings.Contains(logb.String(), "unknown action") || strings.Contains(logb.String(), "meet.google") || strings.Contains(logb.String(), "abc-defg") {
		t.Fatalf("log:\n%s", logb.String())
	}
	if !strings.Contains(logb.String(), "opened the meeting link") {
		t.Errorf("log has no join result:\n%s", logb.String())
	}
	// A stale id (a slot that is no longer offered) opens nothing.
	r.handle(event{kind: evMenu, id: "join:1"})
	if len(u.urls) != 1 {
		t.Fatalf("stale id opened %q", u.urls)
	}
	if !strings.Contains(logb.String(), "that meeting is no longer offered") || strings.Contains(logb.String(), "unknown action") {
		t.Errorf("log:\n%s", logb.String())
	}
}

// An invite the owner has not accepted still offers Join, and only a click
// opens it, even with auto_open on.
func TestRunnerUnansweredInviteOffersJoinOnly(t *testing.T) {
	e := seededEnv(t)
	e.WriteConfig(t, apptest.NoRoutines+"[calendar]\nauto_open = true\n")
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	if err := r.a.Enqueue(unansweredNotice(testMeetLink)); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.posts) != 1 || !strings.Contains(u.posts[0], "\tjoin\tJoin") || len(u.urls) != 0 {
		t.Fatalf("posts %q urls %q, want a Join button and nothing opened", u.posts, u.urls)
	}
	r.handle(event{kind: evResponse, notice: 1, id: "join"})
	if len(u.urls) != 1 || u.urls[0] != testMeetLink {
		t.Fatalf("after the click: urls %q", u.urls)
	}
}

func TestRunnerPostsRecommend(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.FocusHistory(21)...)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices+apptest.AllHours)
	e.Clock = time.Date(2026, 10, 10, 9, 30, 0, 0, apptest.Zone)
	e.Heartbeat(t, e.Clock.UnixMilli())
	// trackd's side: its own App on the same database.
	td, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	defer td.Close()
	evs, err := td.FocusTick(false)
	if err != nil || len(evs) != 1 {
		t.Fatalf("FocusTick = %v, %v, want one note", evs, err)
	}
	if err := td.Enqueue(evs[0]); err != nil {
		t.Fatal(err)
	}

	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.tick()
	if len(u.posts) != 1 {
		t.Fatalf("posts = %q", u.posts)
	}
	f := strings.Split(u.posts[0], "|")
	if len(f) != 4 || f[0] != "Good time to focus" || f[1] != "Your best focus hour · start a 25 min session now?" ||
		!strings.HasPrefix(f[2], "recommend-") {
		t.Fatalf("post = %q", u.posts[0])
	}
	if want := f[2] + "\trec:accept\tStart focus (25 min)\trec:dismiss\tNot now"; f[3] != want {
		t.Fatalf("categories = %q, want %q", f[3], want)
	}
	nid, err := strconv.ParseInt(strings.TrimPrefix(f[2], "recommend-"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	r.handle(event{kind: evResponse, notice: nid, id: "rec:accept"})
	s, err := td.RunningSessionID()
	if err != nil || s == 0 {
		t.Fatalf("running session = %d, %v, want the 25 min session started", s, err)
	}
	db := e.RawDB(t)
	defer db.Close()
	var reply string
	var planned, start int64
	if err := db.QueryRow(`SELECT reply FROM recommend`).Scan(&reply); err != nil || reply != "accept" {
		t.Fatalf("reply = %q (err %v), want accept", reply, err)
	}
	if err := db.QueryRow(`SELECT start_ms, planned_ms FROM session WHERE id = ?`, s).Scan(&start, &planned); err != nil || planned-start != 25*60_000 {
		t.Fatalf("session %d: %d..%d (err %v), want 25 min", s, start, planned, err)
	}
}

func TestRunnerMenuRec(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.FocusHistory(21)...)
	e.Seed(t, apptest.Span{ID: 1, Start: "09:00", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices+apptest.AllHours)
	e.Clock = time.Date(2026, 10, 10, 9, 30, 0, 0, apptest.Zone)
	e.Heartbeat(t, e.Clock.UnixMilli())
	td, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	defer td.Close()
	evs, err := td.FocusTick(false)
	if err != nil || len(evs) != 1 {
		t.Fatalf("FocusTick = %v, %v, want one note", evs, err)
	}

	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.tick()
	menu := last(u.menus)
	key := strconv.FormatInt(evs[0].RecID, 10) + ":" + strconv.FormatInt(e.Clock.UnixMilli(), 10)
	for _, want := range []string{"Suggested: focus now (25 min)", "rec:accept:" + key + "\tStart focus (25 min)", "rec:dismiss:" + key + "\tNot now"} {
		if !strings.Contains(menu, want) {
			t.Fatalf("menu lacks %q:\n%s", want, menu)
		}
	}
	r.handle(event{kind: evMenu, id: "rec:accept:" + key})
	if s, err := td.RunningSessionID(); err != nil || s == 0 {
		t.Fatalf("running session = %d, %v, want the 25 min session", s, err)
	}
	if menu := last(u.menus); strings.Contains(menu, "Suggested") || strings.Contains(menu, "Not now") {
		t.Fatalf("menu after the click still offers the suggestion:\n%s", menu)
	}
}

// TestRunnerShowsBreakScreen: a break screen notice goes to the full-screen
// curtain with its break and buttons, never to a banner; its buttons answer
// like a notification's.
func TestRunnerShowsBreakScreen(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	now := e.Clock.UnixMilli()
	if _, err := r.a.TakeBreak(10 * 60_000); err != nil {
		t.Fatal(err)
	}
	ev := focus.Event{Topic: focus.TopicBreakScreen, Title: focus.TitleBreakScreen, Body: "Your 10 min break ran 5 min over.",
		Break: classify.Interval{From: now - 15*60_000, To: now - 5*60_000}}
	if err := r.a.Enqueue(ev); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.posts) != 0 || len(u.curtains) != 1 {
		t.Fatalf("posts = %q, curtains = %q", u.posts, u.curtains)
	}
	f := strings.Split(u.curtains[0], "|")
	want := []string{focus.TitleBreakScreen, ev.Body, "away", strconv.FormatInt(ev.Break.From, 10), strconv.FormatInt(ev.Break.To, 10),
		"back\tBack to work\tmore\t5 more minutes"}
	if len(f) != 7 || !slices.Equal(f[1:], want) {
		t.Fatalf("curtain = %q", u.curtains[0])
	}
	nid, _ := strconv.ParseInt(f[0], 10, 64)
	r.handle(event{kind: evResponse, notice: nid, id: "more"})
	if until, err := r.a.BreakUntil(e.Clock); err != nil || until != now+5*60_000 {
		t.Fatalf("break until = %d, %v", until, err)
	}
}

// TestRunnerRecapOpensToday: clicking the recap notice itself opens the
// dashboard on Today; another notice's click opens nothing.
func TestRunnerRecapOpensToday(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	for _, ev := range []focus.Event{
		{Topic: focus.TopicRecap, Title: focus.TitleRecap, Body: "4h today."},
		{Topic: focus.TopicBreakOver, Title: focus.TitleBreakOver, Body: "Back to it."},
	} {
		if err := r.a.Enqueue(ev); err != nil {
			t.Fatal(err)
		}
	}
	r.tick()
	if len(u.posts) != 2 {
		t.Fatalf("posts = %q", u.posts)
	}
	ids := r.noticeIDs(t)
	r.handle(event{kind: evResponse, notice: ids[1], id: defaultAction})
	if len(u.urls) != 0 {
		t.Fatalf("Break over opened %q", u.urls)
	}
	r.handle(event{kind: evResponse, notice: ids[0], id: defaultAction})
	if len(u.urls) != 1 || !strings.HasPrefix(u.urls[0], "http://127.0.0.1:") || strings.Contains(u.urls[0], "?v=") {
		t.Fatalf("recap opened %q, want the dashboard on Today", u.urls)
	}
}

// noticeIDs is the ids of the notices queued so far, oldest first.
func (r *runner) noticeIDs(t *testing.T) []int64 {
	t.Helper()
	var ids []int64
	for id := int64(1); id < 100; id++ {
		if _, _, err := r.a.Notice(id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// TestRunnerShowsWakeScreen: the wake notice covers the screens too, its
// clock counting the time off task.
func TestRunnerShowsWakeScreen(t *testing.T) {
	e := seededEnv(t)
	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	s, err := r.a.StartSession(50)
	if err != nil {
		t.Fatal(err)
	}
	from := e.Clock.UnixMilli() - 10*60_000
	ev := focus.Event{Topic: focus.TopicWake, Title: focus.TitleWake, Body: "10 min off task", OffFrom: from, Idle: true, Sess: s}
	if err := r.a.Enqueue(ev); err != nil {
		t.Fatal(err)
	}
	r.tick()
	if len(u.posts) != 0 || len(u.curtains) != 1 {
		t.Fatalf("posts = %q, curtains = %q", u.posts, u.curtains)
	}
	f := strings.Split(u.curtains[0], "|")
	want := []string{focus.TitleWake, ev.Body, "off task", strconv.FormatInt(from, 10), strconv.FormatInt(from, 10),
		"back\tBack to work\twake:paper\tI'm on paper\tsnooze:distract\tSnooze 10 min\tstop\tEnd session"}
	if len(f) != 7 || !slices.Equal(f[1:], want) {
		t.Fatalf("curtain = %q", u.curtains[0])
	}
}
