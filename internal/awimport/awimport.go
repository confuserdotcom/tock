// Package awimport reads an ActivityWatch export (the JSON of "Export all
// buckets" or GET /api/0/export) into views by day. It does no I/O and reads
// no clock.
package awimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"tock/internal/span"
)

// View is one stretch of a window in front.
type View struct {
	StartMS, EndMS int64
	Bundle, App    string
	Title, URL     *string
}

// Day is the views of one local day, Key as YYYY-MM-DD.
type Day struct {
	Key   string
	Views []View
}

type event struct {
	Timestamp time.Time       `json:"timestamp"`
	Duration  float64         `json:"duration"`
	Data      json.RawMessage `json:"data"`
}

type bucket struct {
	Type   string  `json:"type"`
	Events []event `json:"events"`
}

type interval struct{ from, to int64 }

type tab struct {
	interval
	url, title string
}

// ErrEmpty is returned for an export with no window events.
var ErrEmpty = errors.New("awimport: no window events in the export")

// Parse reads an export. Time marked afk is left out, a browser window
// takes the URL and title of the tab that covers most of it, and views are
// split at midnight in loc.
func Parse(data []byte, loc *time.Location) ([]Day, error) {
	var ex struct {
		Buckets map[string]bucket `json:"buckets"`
	}
	if err := json.Unmarshal(data, &ex); err != nil {
		return nil, fmt.Errorf("awimport: read export: %w", err)
	}
	var wins []View
	var afk []interval
	var tabs []tab
	for _, b := range ex.Buckets {
		for _, e := range b.Events {
			iv := interval{e.Timestamp.UnixMilli(), e.Timestamp.UnixMilli() + int64(e.Duration*1000)}
			if iv.to <= iv.from {
				continue
			}
			var d struct{ App, Title, Status, URL string }
			if err := json.Unmarshal(e.Data, &d); err != nil {
				continue
			}
			switch b.Type {
			case "currentwindow":
				if d.App == "" {
					continue
				}
				title := d.Title
				wins = append(wins, View{StartMS: iv.from, EndMS: iv.to, Bundle: d.App, App: d.App, Title: &title})
			case "afkstatus":
				if d.Status == "afk" {
					afk = append(afk, iv)
				}
			case "web.tab.current":
				// Stored like trackd stores it: no scheme, query or fragment.
				if key, ok := span.URLKey(d.URL); ok {
					tabs = append(tabs, tab{iv, key, d.Title})
				}
			}
		}
	}
	if len(wins) == 0 {
		return nil, ErrEmpty
	}
	sort.Slice(wins, func(i, j int) bool { return wins[i].StartMS < wins[j].StartMS })
	var views []View
	for _, w := range wins {
		for _, p := range subtract(interval{w.StartMS, w.EndMS}, afk) {
			v := w
			v.StartMS, v.EndMS = p.from, p.to
			if t, ok := bestTab(p, tabs); ok {
				url, title := t.url, t.title
				v.URL, v.Title = &url, &title
			}
			views = append(views, v)
		}
	}
	return byDay(views, loc), nil
}

// subtract returns iv without the parts covered by cut.
func subtract(iv interval, cut []interval) []interval {
	out := []interval{iv}
	for _, c := range cut {
		var next []interval
		for _, p := range out {
			if c.to <= p.from || c.from >= p.to {
				next = append(next, p)
				continue
			}
			if c.from > p.from {
				next = append(next, interval{p.from, c.from})
			}
			if c.to < p.to {
				next = append(next, interval{c.to, p.to})
			}
		}
		out = next
	}
	return out
}

// bestTab is the tab overlapping iv the most, if it covers at least half.
func bestTab(iv interval, tabs []tab) (tab, bool) {
	var best tab
	var most int64
	for _, t := range tabs {
		o := min(iv.to, t.to) - max(iv.from, t.from)
		if o > most {
			best, most = t, o
		}
	}
	return best, most > 0 && 2*most >= iv.to-iv.from
}

func byDay(views []View, loc *time.Location) []Day {
	var days []Day
	add := func(v View) {
		key := time.UnixMilli(v.StartMS).In(loc).Format(time.DateOnly)
		if n := len(days); n == 0 || days[n-1].Key != key {
			days = append(days, Day{Key: key})
		}
		days[len(days)-1].Views = append(days[len(days)-1].Views, v)
	}
	for _, v := range views {
		for v.EndMS > v.StartMS {
			t := time.UnixMilli(v.StartMS).In(loc)
			next := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc).UnixMilli()
			if v.EndMS <= next {
				add(v)
				break
			}
			head := v
			head.EndMS = next
			add(head)
			v.StartMS = next
		}
	}
	// Views of one day may come out of order around midnight; merge keys.
	sort.SliceStable(days, func(i, j int) bool { return days[i].Key < days[j].Key })
	merged := days[:0]
	for _, d := range days {
		if n := len(merged); n > 0 && merged[n-1].Key == d.Key {
			merged[n-1].Views = append(merged[n-1].Views, d.Views...)
			continue
		}
		merged = append(merged, d)
	}
	return merged
}
