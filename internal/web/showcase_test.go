//go:build demo

package web

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"tock/internal/app"
	"tock/internal/apptest"
)

// showcaseRules is a freelancer's rule set: two clients billed by the hour,
// tock itself, and the usual leaks.
const showcaseRules = `
[[category]]
name = "Code"
kind = "deep"

[[category]]
name = "Design"
kind = "deep"

[[category]]
name = "Writing"
kind = "deep"

[[category]]
name = "Meetings"
kind = "shallow"

[[category]]
name = "Communication"
kind = "shallow"

[[category]]
name = "Research"
kind = "browse"

[[category]]
name = "Social"
kind = "distract"

[[category]]
name = "Video"
kind = "distract"

[[project]]
name = "Atlas"
aliases = ["atlas-ios"]
rate = 95

[[project]]
name = "Northwind"
rate = 120

[[project]]
name = "tock"
category = "Code"

[[project]]
name = "Blog"
category = "Writing"

[[project]]
name = "North-wind"
category = "Design"

[[project]]
name = "Pitch deck"
category = "Design"

[[project]]
name = "Old site"
archived = true

[[rule]]
title = 'Atlas'
project = "Atlas"

[[rule]]
title = 'Northwind'
project = "Northwind"

[[rule]]
title = '^tock'
project = "tock"

[[rule]]
title = 'blog'
project = "Blog"

[[rule]]
bundle = ["com.apple.dt.Xcode", "term"]
category = "Code"

[[rule]]
bundle = ["com.figma.Desktop"]
category = "Design"

[[rule]]
bundle = ["notion.id"]
category = "Writing"

[[rule]]
bundle = ["us.zoom.xos"]
category = "Meetings"

[[rule]]
bundle = ["com.tinyspeck.slackmacgap", "mail"]
category = "Communication"

[[rule]]
url = '^developer\.apple\.com|^stackoverflow\.com|^github\.com'
category = "Research"

[[rule]]
url = '^(x|twitter)\.com|^reddit\.com'
category = "Social"

[[rule]]
url = '^(www\.)?youtube\.com'
category = "Video"

[[goal]]
category = "Code"
at_least = "3h"

[[goal]]
category = "Design"
at_least = "8h"
per = "week"

[[goal]]
category = "Social"
at_most = "30m"
`

// block is one stretch of a showcase day: from and to as minutes after
// midnight, what was in front.
type block struct {
	from, to    int
	bundle, app string
	title, url  string
}

func hm(h, m int) int { return h*60 + m }

// showcaseToday is the day the video shows, a Thursday up to 17:20.
func showcaseToday() []block {
	return []block{
		{hm(8, 52), hm(9, 6), "mail", "Mail", "Inbox (12)", ""},
		{hm(9, 6), hm(9, 18), "com.tinyspeck.slackmacgap", "Slack", "#atlas", ""},
		{hm(9, 18), hm(10, 2), "com.apple.dt.Xcode", "Xcode", "Atlas — PlayerView.swift", ""},
		{hm(10, 2), hm(10, 5), "com.tinyspeck.slackmacgap", "Slack", "#atlas", ""},
		{hm(10, 5), hm(11, 4), "com.apple.dt.Xcode", "Xcode", "Atlas — Playback.swift", ""},
		{hm(11, 4), hm(11, 13), "arc", "Arc", "Video", "www.youtube.com/watch"},
		{hm(11, 13), hm(11, 30), "arc", "Arc", "AVPlayer | Apple Developer", "developer.apple.com/documentation/avfoundation"},
		{hm(11, 30), hm(12, 2), "us.zoom.xos", "zoom.us", "Atlas weekly", ""},
		{hm(12, 2), hm(12, 51), "tock.idle", "Idle", "", ""},
		{hm(12, 51), hm(13, 4), "arc", "Arc", "Home / X", "x.com/home"},
		{hm(13, 4), hm(14, 41), "com.figma.Desktop", "Figma", "Northwind — Checkout flow", ""},
		{hm(14, 41), hm(14, 53), "arc", "Arc", "Spacing scale for dense UI", "chatgpt.com/c/7"},
		{hm(14, 53), hm(15, 9), "arc", "Arc", "Dribbble - checkout", "dribbble.com/shots/2214"},
		{hm(15, 9), hm(16, 31), "term", "WezTerm", "tock │ nvim notch.m", ""},
		{hm(16, 31), hm(16, 44), "com.tinyspeck.slackmacgap", "Slack", "#northwind", ""},
		{hm(16, 44), -1, "notion.id", "Notion", "Northwind — Spec v3", ""},
	}
}

// showcaseDay makes up a past day: mornings in Xcode or Figma, a meeting,
// a lunch away, an afternoon on a second project, and some leaks.
func showcaseDay(r *rand.Rand, wd time.Weekday) []block {
	if wd == time.Sunday || (wd == time.Saturday && r.Intn(3) > 0) {
		if r.Intn(2) == 0 {
			return nil
		}
		s := hm(10+r.Intn(2), r.Intn(50))
		return []block{
			{s, s + 70 + r.Intn(60), "term", "WezTerm", "tock │ nvim", ""},
			{s + 140, s + 175, "arc", "Arc", "Video", "www.youtube.com/watch"},
		}
	}
	j := func(n int) int { return r.Intn(n) }
	t := hm(8, 40+j(35))
	var out []block
	add := func(d int, bundle, app, title, url string) {
		out = append(out, block{t, t + d, bundle, app, title, url})
		t += d
	}
	add(8+j(10), "mail", "Mail", "Inbox", "")
	add(6+j(12), "com.tinyspeck.slackmacgap", "Slack", "#atlas", "")
	if r.Intn(5) < 3 {
		add(60+j(70), "com.apple.dt.Xcode", "Xcode", "Atlas — Player.swift", "")
	} else {
		add(60+j(70), "com.figma.Desktop", "Figma", "Northwind — Components", "")
	}
	if r.Intn(3) == 0 {
		add(5+j(20), "arc", "Arc", "Video", "www.youtube.com/watch")
	}
	add(10+j(25), "arc", "Arc", "Docs", "developer.apple.com/documentation")
	if wd == time.Monday || wd == time.Thursday {
		add(30, "us.zoom.xos", "zoom.us", "Atlas weekly", "")
	}
	add(25+j(50), "com.apple.dt.Xcode", "Xcode", "Atlas — Tests", "")
	add(40+j(25), "tock.idle", "Idle", "", "")
	add(5+j(20), "arc", "Arc", "Home / X", "x.com/home")
	if r.Intn(2) == 0 {
		add(60+j(60), "com.figma.Desktop", "Figma", "Northwind — Checkout flow", "")
	} else {
		add(50+j(50), "notion.id", "Notion", "Northwind — Spec", "")
	}
	add(6+j(10), "com.tinyspeck.slackmacgap", "Slack", "#northwind", "")
	add(50+j(70), "term", "WezTerm", "tock │ make test", "")
	if r.Intn(3) == 0 {
		add(10+j(25), "arc", "Arc", "r/macapps", "reddit.com/r/macapps")
	}
	if r.Intn(4) == 0 {
		add(30+j(30), "notion.id", "Notion", "blog: what I learned", "")
	}
	return out
}

// TestShowcase serves a month of made-up freelancer data for the README
// videos and screenshots:
//
//	TOCK_DEMO_PORT=7799 go test -tags demo -run TestShowcase -timeout 0 ./internal/web
func TestShowcase(t *testing.T) {
	e := apptest.New(t, showcaseRules)
	e.WriteConfig(t, "[invoice]\nname = \"Sam Rivera\"\ncurrency = \"EUR\"\n")
	now := time.Date(2026, 11, 5, 17, 20, 0, 0, apptest.Zone)
	e.Clock = now
	db := e.RawDB(t)
	defer db.Close()
	id := int64(0)
	put := func(day time.Time, bs []block) {
		for _, b := range bs {
			id++
			start := day.Add(time.Duration(b.from) * time.Minute).UnixMilli()
			var end any
			if b.to >= 0 {
				end = day.Add(time.Duration(b.to) * time.Minute).UnixMilli()
			}
			var title, url any
			if b.title != "" {
				title = b.title
			}
			if b.url != "" {
				url = b.url
			}
			if _, err := db.Exec(`INSERT INTO span(id, start, "end", bundle_id, app, title, url, private) VALUES(?,?,?,?,?,?,?,0)`,
				id, start, end, b.bundle, b.app, title, url); err != nil {
				t.Fatal(err)
			}
		}
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, apptest.Zone)
	r := rand.New(rand.NewSource(7))
	for d := 42; d >= 1; d-- {
		day := midnight.AddDate(0, 0, -d)
		put(day, showcaseDay(r, day.Weekday()))
	}
	put(midnight, showcaseToday())
	at := func(h, m int) int64 { return midnight.Add(time.Duration(hm(h, m)) * time.Minute).UnixMilli() }
	if _, err := db.Exec(`INSERT INTO call(start_ms, end_ms) VALUES(?, ?)`, at(11, 30), at(12, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO event(start_ms, end_ms, title, calendar, join_url, mine) VALUES(?, ?, 'Atlas weekly', 'Work', '', 1)`, at(11, 30), at(12, 0)); err != nil {
		t.Fatal(err)
	}
	day := now.Format("2006-01-02")
	e.Summary(t, day, 0, "A long morning in Atlas playback, the Northwind checkout flow after lunch, then tock's notch until five.")
	e.Heartbeat(t, now.UnixMilli())
	a, err := app.Open(app.Paths{DB: e.DB, Rules: e.Rules, Learned: e.Learned, Config: e.Config, ShellLog: e.ShellLog}, e.NowFunc())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	port := os.Getenv("TOCK_DEMO_PORT")
	if port == "" {
		port = "7799"
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("%064d", 0)
	fmt.Printf("showcase: http://127.0.0.1:%d/#t=%s\n", ln.Addr().(*net.TCPAddr).Port, token)
	t.Fatal(http.Serve(ln, New(a, token, ln.Addr().(*net.TCPAddr).Port, e.NowFunc()).Handler()))
}
