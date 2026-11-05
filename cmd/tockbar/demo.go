package main

import (
	"strconv"
	"strings"
	"time"

	"tock/internal/app"
	"tock/internal/bar"
	"tock/internal/classify"
	"tock/internal/focus"
	"tock/internal/report"
	"tock/internal/stats"
	"tock/internal/store"
)

// demoStep is one island of the notch demo and how long it stays.
type demoStep struct {
	island func(now time.Time) bar.Island
	stay   time.Duration
}

// demoEntries are the made-up inbox of the demo.
func demoEntries() []bar.Entry {
	return []bar.Entry{
		{App: "Figma", Title: "tock-notch.fig", Sub: "Figma · 3 entries", Dur: "42m", MS: 42 * 60_000, Picked: -1,
			Opts: []bar.Option{{Label: "Design / tock", Kind: "deep", Pct: 86}, {Label: "Meetings", Kind: "shallow"}, {Label: "Browsing", Kind: "browse"}}},
		{App: "Safari", Title: "Hacker News", Sub: "news.ycombinator.com · 9 entries", Dur: "18m", MS: 18 * 60_000, Picked: -1,
			Opts: []bar.Option{{Label: "Browsing", Kind: "browse", Pct: 71}, {Label: "Research / tock", Kind: "shallow"}, {Label: "Entertainment", Kind: "distract"}}},
		{App: "Terminal", Title: "ssh ryuk", Sub: "Terminal · 4 entries", Dur: "25m", MS: 25 * 60_000, Picked: -1,
			Opts: []bar.Option{{Label: "Admin / ryuk", Kind: "shallow", Pct: 64}, {Label: "Code / tock", Kind: "deep"}, {Label: "Neutral", Kind: "neutral"}}},
	}
}

func demoSteps() []demoStep {
	ms := func(t time.Time) int64 { return t.UnixMilli() }
	session := func(now time.Time) *focus.Session {
		return &focus.Session{ID: 1, StartMS: ms(now) - 6*60_000, PlannedMS: ms(now) + 19*60_000, Goal: "tock notch"}
	}
	quiet := func(now time.Time) bar.Island {
		s := bar.Snapshot{OK: true, TrackedMS: 4*3600_000 + 12*60_000, Score: 78, Active: true, UnlabelledMS: 85 * 60_000}
		i := s.IslandBase(ms(now))
		d, w, u := demoDay(now)
		i.Hover.Pages = bar.TodayPages(s, d, w, u, ms(now))
		return i
	}
	done := func(now time.Time) bar.Island {
		end := ms(now)
		return bar.DoneIsland(store.Notice{ID: 3, Topic: focus.TopicDone, Title: "Session done"}, app.NoticeData{Session: 1, Minutes: 25},
			&report.SessionDetail{Sess: focus.Session{ID: 1, StartMS: end - 25*60_000, PlannedMS: end, EndMS: end,
				DeepMS: 21 * 60_000, LongestMS: 14 * 60_000, Switches: 3, DistractMS: 2 * 60_000, Goal: "tock notch"}},
			time.Local, 85*60_000)
	}
	return []demoStep{
		{quiet, 4 * time.Second},
		{func(now time.Time) bar.Island {
			return bar.Snapshot{OK: true, Session: session(now), Live: focus.Live{Level: 84}}.IslandBase(ms(now))
		}, 5 * time.Second},
		{func(now time.Time) bar.Island {
			return bar.DriftIsland(store.Notice{ID: 2, Topic: focus.TopicFocus, Title: "Back to tock notch?",
				Body: "YouTube has had the last 2 minutes of this session."},
				app.NoticeData{Session: 1, SnoozeMin: 15, Category: "YouTube"})
		}, 6 * time.Second},
		{func(now time.Time) bar.Island {
			return bar.Snapshot{OK: true, Session: session(now), Live: focus.Live{Level: 71}}.IslandBase(ms(now))
		}, 3 * time.Second},
		{done, 6 * time.Second},
		{func(time.Time) bar.Island { return bar.LabelIsland(demoEntries(), 0, "") }, 0}, // until done
		{func(now time.Time) bar.Island {
			return bar.Snapshot{OK: true, Slots: []focus.Slot{{Kind: focus.SlotEvent, StartMS: ms(now) + 2*60_000, EndMS: ms(now) + 15*60_000,
				Title: "Standup", Calendar: "Work"}}}.IslandBase(ms(now))
		}, 5 * time.Second},
		{func(now time.Time) bar.Island {
			day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
			at := func(h, m int) int64 {
				return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).UnixMilli()
			}
			kinds := []string{"shallow", "deep", "deep", "browse", "deep", "neutral", "shallow", "deep", "deep", "distract", "shallow", "deep", "browse", "deep"}
			var rows []report.Row
			for i, k := range kinds {
				rows = append(rows, report.Row{Start: at(9, 0) + int64(i)*45*60_000, End: at(9, 0) + int64(i+1)*45*60_000, Label: classify.Label{Kind: k}})
			}
			d := report.Day{Date: day, TrackedMS: 6*3600_000 + 48*60_000, Score: stats.Score{Value: 81},
				Stats:    stats.Day{FocusMS: 192 * 60_000, LongestMS: 94 * 60_000, DistractMS: 14 * 60_000},
				Projects: []report.Share{{Name: "tock", MS: 242 * 60_000}, {Name: "Licence", MS: 91 * 60_000}, {Name: "", MS: 75 * 60_000}},
				Timeline: rows}
			return bar.RecapIsland(store.Notice{ID: 4, Topic: focus.TopicRecap}, d,
				app.Usual{Days: 4, Score: 75, FocusMS: 168 * 60_000, LongestMS: 76 * 60_000, DistractMS: 23 * 60_000}, 85*60_000, time.Local)
		}, 8 * time.Second},
		// a session end nobody saw waits as a dot
		{func(now time.Time) bar.Island { return bar.Missed(quiet(now), done(now)) }, 4 * time.Second},
	}
}

// demoDay is a made-up day and week for the demo's notch pages.
func demoDay(now time.Time) (report.Day, report.Week, app.Usual) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	at := func(h, m int) int64 {
		return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute).UnixMilli()
	}
	kinds := []string{"shallow", "deep", "deep", "browse", "deep", "neutral", "shallow", "deep", "deep", "distract", "shallow", "deep", "browse", "deep"}
	var rows []report.Row
	hours := map[string][24]int64{}
	for i, k := range kinds {
		start := at(9, 0) + int64(i)*45*60_000
		rows = append(rows, report.Row{Start: start, End: start + 45*60_000, Label: classify.Label{Kind: k}})
		h := hours[k]
		h[time.UnixMilli(start).In(time.Local).Hour()] += 45 * 60_000
		hours[k] = h
	}
	d := report.Day{Date: day, TrackedMS: 6*3600_000 + 48*60_000, Score: stats.Score{Value: 81},
		Stats:    stats.Day{FocusMS: 192 * 60_000, LongestMS: 94 * 60_000, DistractMS: 14 * 60_000},
		Hours:    report.Heatmap{First: 9, Last: 19, MS: hours},
		Projects: []report.Share{{Name: "tock", MS: 242 * 60_000}, {Name: "Licence", MS: 91 * 60_000}, {Name: "", MS: 75 * 60_000}},
		Timeline: rows,
		Goals: []report.GoalRow{
			{Goal: classify.Goal{Project: "tock", MS: 3 * 3600_000}, DoneMS: 242 * 60_000, State: "met", Streak: 4},
			{Goal: classify.Goal{Category: "Study", MS: 2 * 3600_000}, DoneMS: 50 * 60_000, State: "short"},
			{Goal: classify.Goal{Category: "Entertainment", AtMost: true, MS: 45 * 60_000}, DoneMS: 14 * 60_000, State: "ok", Streak: 2},
		}}
	w := report.Week{Monday: report.MondayOf(day), ByKind: map[string]int64{}}
	for i := range w.Days {
		w.Days[i].Date = w.Monday.AddDate(0, 0, i)
		w.Days[i].Future = w.Days[i].Date.After(day)
		if w.Days[i].Future || i > 4 {
			continue
		}
		by := map[string]int64{"deep": int64(2+i%3) * 3600_000, "shallow": int64(1+i%2) * 3600_000, "distract": int64(20+10*i) * 60_000, "": 30 * 60_000}
		w.Days[i].ByKind, w.Days[i].Score = by, stats.Score{Value: 64 + 5*i}
		for k, ms := range by {
			w.Days[i].TrackedMS += ms
			w.ByKind[k] += ms
			w.TrackedMS += ms
		}
	}
	return d, w, app.Usual{Days: 4, Score: 75, FocusMS: 168 * 60_000}
}

// runDemo plays the demo islands in a loop on u, for recordings: no
// database, nothing saved. Clicks move it on.
func runDemo(u ui, events <-chan event) {
	steps := demoSteps()
	ready := false
	for !ready {
		if ev := <-events; ev.kind == evNotch && ev.id == "1" {
			ready = true
		}
	}
	u.SetTitle("◷ demo")
	u.SetMenu("quit\tQuit the demo")
	u.NotchOptions(true, true, false) // the demo exists to be recorded
	step, at := 0, 0
	entries := demoEntries()
	var timer <-chan time.Time
	show := func() {
		var i bar.Island
		if steps[step].stay == 0 {
			haptic := ""
			if at > 0 && entries[at-1].Saved != "" {
				haptic = bar.HapticAlignment
			}
			i = bar.LabelIsland(entries, at, haptic)
		} else {
			i = steps[step].island(time.Now())
		}
		u.Island(i.JSON())
		stay := steps[step].stay
		if stay == 0 {
			stay = 3 * time.Second // labelling: pick the guess by itself after a pause
		}
		timer = time.After(stay)
	}
	next := func() {
		step = (step + 1) % len(steps)
		if steps[step].stay == 0 {
			entries, at = demoEntries(), 0
		}
		show()
	}
	pick := func(j int) {
		e := &entries[at]
		e.Picked = j
		e.Saved = "Rule saved: " + e.App + " titles like “" + e.Title + "” → " + e.Opts[j].Label + ". Labels 14 entries, 3h05."
		at++
	}
	show()
	for {
		select {
		case <-timer:
			if steps[step].stay == 0 && at < len(entries) {
				pick(0)
				show()
				continue
			}
			next()
		case ev := <-events:
			if ev.kind != evMenu {
				continue
			}
			switch id := ev.id; {
			case id == "quit":
				u.Quit()
				return
			case strings.HasPrefix(id, bar.IDNotchPick):
				parts := strings.Split(strings.TrimPrefix(id, bar.IDNotchPick), ":")
				if j, err := strconv.Atoi(parts[len(parts)-1]); err == nil && at < len(entries) && j < len(entries[at].Opts) {
					pick(j)
					show()
				}
			case id == bar.IDNotchOpen:
				timer = nil // swiping the quiet panel's pages: stay on it
			case id == bar.IDNotchShut:
				timer = time.After(2 * time.Second)
			case strings.HasPrefix(id, bar.IDNotchSkip):
				if at < len(entries) {
					at++
					show()
				}
			case strings.HasPrefix(id, bar.IDNotch):
				next()
			}
		}
	}
}
