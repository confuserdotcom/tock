package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
)

// slotDayRules is apptest.Rules plus the module the plan item is tagged with.
const slotDayRules = apptest.Rules + `
[[project]]
name = "Analyse 1"
category = "Study"
aliases = ["ana1"]
`

// TestSlotDay runs a day with a plan item and a meeting through trackd's tick
// (FocusTick, then Enqueue, as cmd/trackd does when the bar is ready) and the
// real tockbar runner: every notice comes once, with its buttons, a trackd
// restart repeats nothing, and Join and auto_open open the meeting link.
func TestSlotDay(t *testing.T) {
	const link = "https://meet.google.com/abc-defg-hij"
	e := apptest.New(t, slotDayRules)
	todo := filepath.Join(t.TempDir(), "todo.txt")
	if err := os.WriteFile(todo, []byte("TD 1 +ana1 @14h00 due:2026-10-10\nWrap up @14h30 due:2026-10-10\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.WriteConfig(t, apptest.NoRoutines+"[focus]\nbreak_after = 480\n[plan]\nfile = '"+todo+"'\n[calendar]\nauto_open = true\n")
	e.EventLink(t, "15:00", "15:30", "Standup", link)
	e.Seed(t, apptest.Span{ID: 1, Start: "13:50", Bundle: "term", App: "WezTerm", Title: "x"})
	clock := func(h, m, s int) { e.Clock = time.Date(2026, 10, 10, h, m, s, 0, apptest.Zone) }
	clock(13, 54, 0)
	e.Heartbeat(t, e.Clock.UnixMilli())

	r, u, logb := testRunner(t, e)
	r.handle(event{kind: evReady})
	r.handle(event{kind: evAuth, id: app.NotifyGranted})
	e.Clock = e.Clock.Add(beatEvery)
	r.tick() // this beat tells trackd the bar can show notices
	clock(13, 54, 0)

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
	// post splits a post into title, body, category and category line.
	post := func(p string) (title, body, cat, cats string) {
		f := strings.SplitN(p, "|", 4)
		if len(f) != 4 {
			t.Fatalf("post %q", p)
		}
		return f[0], f[1], f[2], f[3]
	}
	// actions are the buttons of category cat in a category line, which lists
	// the categories of every notice posted so far.
	actions := func(cats, cat string) []string {
		var out []string
		for _, l := range strings.Split(cats, "\n") {
			if rest, ok := strings.CutPrefix(l, cat+"\t"); ok {
				out = append(out, rest)
			}
		}
		return out
	}
	noPost := func(label string, got []string) {
		t.Helper()
		if len(got) != 0 {
			t.Fatalf("%s: posts = %q, want none", label, got)
		}
	}
	one := func(label string, got []string) (title, body, cat, cats string) {
		t.Helper()
		if len(got) != 1 {
			t.Fatalf("%s: posts = %q, want one", label, got)
		}
		return post(got[0])
	}
	noteID := func(cat, prefix string) int64 {
		t.Helper()
		id, err := strconv.ParseInt(strings.TrimPrefix(cat, prefix), 10, 64)
		if err != nil || !strings.HasPrefix(cat, prefix) {
			t.Fatalf("category %q, want %s<id>", cat, prefix)
		}
		return id
	}

	// The plan item: up next at 13:55, starting at 14:00.
	noPost("13:54", at(trackd, 13, 54, 0))
	title, body, cat, cats := one("13:55", at(trackd, 13, 55, 0))
	if title != "Up next" || body != "14:00 TD 1 · in 5 min" || cat != "" || cats != "" {
		t.Fatalf("13:55 post = %q, want Up next without a category", u.posts[seen-1])
	}
	title, body, cat, cats = one("14:00:10", at(trackd, 14, 0, 10))
	if title != "Starting now" || body != "14:00–14:30 TD 1" || !strings.HasPrefix(cat, "starting-") ||
		!slices.Equal(actions(cats, cat), []string{"again\tStart focus (30 min)"}) {
		t.Fatalf("14:00:10 post = %q, want the starting notice with Start focus (30 min)", u.posts[seen-1])
	}
	r.handle(event{kind: evResponse, notice: noteID(cat, "starting-"), id: "again"})
	f, err := r.a.Focus(e.Clock)
	if err != nil || f.Open == nil || f.Open.PlannedMS-f.Open.StartMS != 30*60_000 {
		t.Fatalf("after Start focus: open %+v, err %v, want a 30 min session", f.Open, err)
	}
	if _, err := r.a.StopSession(); err != nil {
		t.Fatal(err)
	}
	noPost("14:00:20", at(trackd, 14, 0, 20))
	noPost("14:01 after a trackd restart", at(open(), 14, 1, 0))

	// The meeting: up next with Join, starting with Join and auto_open.
	title, body, cat, cats = one("14:55", at(trackd, 14, 55, 0))
	if title != "Up next" || body != "15:00 Standup (Work) · in 5 min" || !strings.HasPrefix(cat, "upcoming-") ||
		!slices.Equal(actions(cats, cat), []string{"join\tJoin"}) {
		t.Fatalf("14:55 post = %q, want Up next with Join", u.posts[seen-1])
	}
	if len(u.urls) != 0 {
		t.Fatalf("up next opened %q", u.urls)
	}
	title, body, cat, cats = one("15:00:10", at(trackd, 15, 0, 10))
	if title != "Starting now" || body != "15:00–15:30 Standup (Work)" || !strings.HasPrefix(cat, "starting-") ||
		len(actions(cats, cat)) != 1 || actions(cats, cat)[0] != "join\tJoin" {
		t.Fatalf("15:00:10 post = %q, want Starting now with Join first", u.posts[seen-1])
	}
	if len(u.urls) != 1 || u.urls[0] != link {
		t.Fatalf("auto_open: urls = %q, want the link once", u.urls)
	}
	noPost("15:00:20", at(trackd, 15, 0, 20))
	if len(u.urls) != 1 {
		t.Fatalf("urls after 15:00:20 = %q, want no new one", u.urls)
	}
	r.handle(event{kind: evResponse, notice: noteID(cat, "starting-"), id: "join"})
	if len(u.urls) != 2 || u.urls[1] != link {
		t.Fatalf("after Join: urls = %q, want the link twice", u.urls)
	}
	if strings.Contains(logb.String(), "meet.google") || strings.Contains(logb.String(), "abc-defg") {
		t.Fatalf("tockbar log holds the link:\n%s", logb.String())
	}
}
