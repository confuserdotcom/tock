package report

import (
	"slices"
	"sort"
	"strings"

	"tock/internal/classify"
	"tock/internal/span"
)

// AgentSession is one coding agent session filed under a project (or its
// directory when no project's repo holds it).
type AgentSession struct {
	ID, Project string
	Titles      []string
	Events      []int64 // ascending
	Prompts     []int64
}

// AgentRow is one project's agent time and your time in its windows.
type AgentRow struct {
	Project           string
	AgentMS, YouMS    int64
	Prompts, Sessions int
}

// Agents is the agent ledger of a range: per project, most agent time
// first, and the totals. WallMS is the time at least one agent worked; Peak
// the most sessions working at once.
type Agents struct {
	Rows                   []AgentRow
	AgentMS, WallMS, YouMS int64
	Peak                   int
}

// AgentLedger measures what agents did in [from, to). A session works from
// one event to the next when they are at most gapMS apart; a longer gap is
// waiting for you. Your time is the non-idle views whose title holds one of
// the session's titles (coding agents put them in the terminal title), each
// view counted once.
func AgentLedger(sessions []AgentSession, views []classify.SpanView, labels []classify.Label, from, to, gapMS int64) Agents {
	var a Agents
	rows := map[string]*AgentRow{}
	var all [][2]int64
	for _, s := range sessions {
		r := rows[s.Project]
		if r == nil {
			r = &AgentRow{Project: s.Project}
			rows[s.Project] = r
		}
		busy := busySpans(s.Events, from, to, gapMS)
		if len(busy) == 0 && !anyIn(s.Prompts, from, to) {
			continue
		}
		r.Sessions++
		for _, b := range busy {
			r.AgentMS += b[1] - b[0]
		}
		for _, p := range s.Prompts {
			if p >= from && p < to {
				r.Prompts++
			}
		}
		all = append(all, busy...)
	}
	used := make([]bool, len(views))
	for _, s := range sessions {
		r := rows[s.Project]
		for i, v := range views {
			if used[i] || v.BundleID == span.IdleBundleID || v.Title == nil || !titleHas(*v.Title, s.Titles) {
				continue
			}
			if st, e := max(v.Start, from), min(v.End, to); e > st {
				used[i] = true
				r.YouMS += e - st
			}
		}
	}
	for _, r := range rows {
		if r.Sessions == 0 && r.YouMS == 0 {
			continue
		}
		a.Rows = append(a.Rows, *r)
		a.AgentMS += r.AgentMS
		a.YouMS += r.YouMS
	}
	sort.Slice(a.Rows, func(i, j int) bool {
		if a.Rows[i].AgentMS != a.Rows[j].AgentMS {
			return a.Rows[i].AgentMS > a.Rows[j].AgentMS
		}
		return a.Rows[i].Project < a.Rows[j].Project
	})
	a.WallMS, a.Peak = sweep(all)
	return a
}

// busySpans merges events in [from, to) no more than gapMS apart.
func busySpans(events []int64, from, to, gapMS int64) [][2]int64 {
	var out [][2]int64
	for _, t := range events {
		if t < from || t >= to {
			continue
		}
		if n := len(out); n > 0 && t-out[n-1][1] <= gapMS {
			out[n-1][1] = t
			continue
		}
		out = append(out, [2]int64{t, t})
	}
	return slices.DeleteFunc(out, func(b [2]int64) bool { return b[1] == b[0] })
}

func anyIn(ts []int64, from, to int64) bool {
	return slices.ContainsFunc(ts, func(t int64) bool { return t >= from && t < to })
}

// titleHas reports whether title holds one of titles; short titles are
// ignored, they would match by chance.
func titleHas(title string, titles []string) bool {
	for _, t := range titles {
		if len([]rune(t)) >= 4 && strings.Contains(title, t) {
			return true
		}
	}
	return false
}

// sweep returns the length of the union of spans and the most that overlap.
func sweep(spans [][2]int64) (wall int64, peak int) {
	type ev struct {
		at int64
		d  int
	}
	evs := make([]ev, 0, 2*len(spans))
	for _, s := range spans {
		evs = append(evs, ev{s[0], 1}, ev{s[1], -1})
	}
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].at != evs[j].at {
			return evs[i].at < evs[j].at
		}
		return evs[i].d < evs[j].d // an end before a start at the same time
	})
	n := 0
	var last int64
	for _, e := range evs {
		if n > 0 {
			wall += e.at - last
		}
		n += e.d
		peak = max(peak, n)
		last = e.at
	}
	return wall, peak
}
