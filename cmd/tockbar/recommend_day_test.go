package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
)

// TestRecommendDay runs a day with a focus suggestion and an adaptive break
// through trackd's tick (FocusTick, then Enqueue, as cmd/trackd does when the
// bar is ready) and the real tockbar runner: each notice comes once with its
// buttons, Start focus starts the session, Take a break shows in the menu and
// ends with one Break over, and a trackd restart repeats nothing.
func TestRecommendDay(t *testing.T) {
	e := apptest.New(t, apptest.Rules)
	e.Seed(t, apptest.FocusHistory(21)...)
	// Open since 09:25, so the user's own activity reaches the 5 minutes a
	// suggestion needs at exactly 09:30.
	e.Seed(t, apptest.Span{ID: 1, Start: "09:25", Bundle: "term", App: "WezTerm", Title: "tock │ x"})
	e.WriteConfig(t, apptest.NoRoutines+apptest.NoPlanNotices+apptest.AllHours)
	clock := func(h, m, s int) { e.Clock = time.Date(2026, 10, 10, h, m, s, 0, apptest.Zone) }
	clock(9, 20, 0)
	e.Heartbeat(t, e.Clock.UnixMilli())

	r, u, _ := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evAuth, id: app.NotifyGranted})
	e.Clock = e.Clock.Add(beatEvery)
	r.tick() // this beat tells trackd the bar can show notices
	clock(9, 20, 0)

	open := func() *app.App {
		t.Helper()
		a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config}, e.NowFunc())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { a.Close() })
		return a
	}
	trackd := open()
	if !trackd.BarReady() {
		t.Fatal("trackd does not see a ready bar")
	}
	seen := 0
	// at moves the clock, runs trackd's tick, hands its notes to the bar and
	// runs the bar's tick; it returns the posts that tick added.
	at := func(a *app.App, h, m, s int) []string {
		t.Helper()
		clock(h, m, s)
		evs, err := a.FocusTick(false)
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range evs {
			if err := a.Enqueue(ev); err != nil {
				t.Fatal(err)
			}
		}
		r.tick()
		got := u.posts[seen:]
		seen = len(u.posts)
		return got
	}
	one := func(label string, got []string) (title, body, cat, cats string) {
		t.Helper()
		if len(got) != 1 {
			t.Fatalf("%s: posts = %q, want one", label, got)
		}
		f := strings.SplitN(got[0], "|", 4)
		if len(f) != 4 {
			t.Fatalf("%s: post %q", label, got[0])
		}
		return f[0], f[1], f[2], f[3]
	}
	noPost := func(label string, got []string) {
		t.Helper()
		if len(got) != 0 {
			t.Fatalf("%s: posts = %q, want none", label, got)
		}
	}
	// actions are the buttons of category cat in a category line, which lists
	// the categories of every notice posted so far.
	actions := func(cats, cat string) []string {
		var out []string
		for _, l := range strings.Split(cats, "\n") {
			if rest, ok := strings.CutPrefix(l, cat+"\t"); ok {
				f := strings.Split(rest, "\t") // id, title, id, title ...
				for i := 0; i+1 < len(f); i += 2 {
					out = append(out, f[i]+"|"+f[i+1])
				}
			}
		}
		return out
	}
	noticeID := func(cat, prefix string) int64 {
		t.Helper()
		id, ok := strings.CutPrefix(cat, prefix)
		if !ok {
			t.Fatalf("category %q, want %s<id>", cat, prefix)
		}
		var n int64
		for _, c := range id {
			if c < '0' || c > '9' {
				t.Fatalf("category %q, want %s<id>", cat, prefix)
			}
			n = n*10 + int64(c-'0')
		}
		return n
	}
	lastMenu := func() string { return last(u.menus) }

	// The suggestion: 4 min of own activity is not enough, 5 is.
	noPost("09:29", at(trackd, 9, 29, 0))
	title, body, cat, cats := one("09:30", at(trackd, 9, 30, 0))
	if title != "Good time to focus" || body != "Your best focus hour · start a 25 min session now?" ||
		!strings.HasPrefix(cat, "recommend-") ||
		!slices.Equal(actions(cats, cat), []string{"rec:accept|Start focus (25 min)", "rec:dismiss|Not now"}) {
		t.Fatalf("09:30 post = %q, want the suggestion with Start focus (25 min) and Not now", u.posts[seen-1])
	}
	if !strings.Contains(lastMenu(), "Suggested: focus now (25 min)") {
		t.Fatalf("menu lacks the suggestion:\n%s", lastMenu())
	}
	r.handle(event{kind: evResponse, notice: noticeID(cat, "recommend-"), id: "rec:accept"})
	f, err := r.a.Focus(e.Clock)
	if err != nil || f.Open == nil || f.Open.PlannedMS-f.Open.StartMS != 25*60_000 {
		t.Fatalf("after Start focus: open %+v, err %v, want a 25 min session", f.Open, err)
	}
	if strings.Contains(lastMenu(), "Suggested:") {
		t.Fatalf("menu still offers the answered suggestion:\n%s", lastMenu())
	}
	noPost("09:30:10", at(trackd, 9, 30, 10))
	noPost("09:31 after a trackd restart", at(open(), 9, 31, 0))

	// The session ends; nothing else is due in the hour.
	title, _, _, _ = one("09:55", at(trackd, 9, 55, 0))
	if title != "tock · session done" {
		t.Fatalf("09:55 post = %q, want the session done notice", u.posts[seen-1])
	}
	noPost("10:00", at(trackd, 10, 0, 0))

	// The adaptive break: 60 min of sustained focus, so the reminder comes after
	// 60 min of activity (09:25) and suggests a 10 min break.
	noPost("10:24", at(trackd, 10, 24, 0))
	title, body, cat, cats = one("10:25", at(trackd, 10, 25, 0))
	if title != "tock · break" || body != "60 min without a break. Stand up for 10 min." || !strings.HasPrefix(cat, "break-") ||
		!slices.Equal(actions(cats, cat), []string{"rec:accept|Take a break (10 min)", "rec:dismiss|Not now", "snooze|Snooze 30 min"}) {
		t.Fatalf("10:25 post = %q, want the break reminder with Take a break (10 min) first", u.posts[seen-1])
	}
	r.handle(event{kind: evResponse, notice: noticeID(cat, "break-"), id: "rec:accept"})
	if !strings.Contains(lastMenu(), "Break · until 10:35") {
		t.Fatalf("menu lacks the running break:\n%s", lastMenu())
	}
	noPost("10:30", at(trackd, 10, 30, 0))
	noPost("10:34", at(trackd, 10, 34, 0))
	title, body, cat, _ = one("10:35:02", at(trackd, 10, 35, 2))
	if title != "Break over" || body != "10 min break done. Back to it." || cat != "" {
		t.Fatalf("10:35:02 post = %q, want Break over without buttons", u.posts[seen-1])
	}
	if strings.Contains(lastMenu(), "Break ·") {
		t.Fatalf("menu still shows the break after it ended:\n%s", lastMenu())
	}
	noPost("10:35:07", at(trackd, 10, 35, 7))
	noPost("10:36 after a trackd restart", at(open(), 10, 36, 0))

	// Both notices were answered with accept, each recorded once.
	db := e.RawDB(t)
	defer db.Close()
	rows, err := db.Query(`SELECT kind, reply FROM recommend ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var kind, reply string
		if err := rows.Scan(&kind, &reply); err != nil {
			t.Fatal(err)
		}
		got = append(got, kind+" "+reply)
	}
	if want := []string{"focus accept", "break accept"}; !slices.Equal(got, want) {
		t.Fatalf("recommend rows = %q, want %q", got, want)
	}
}
