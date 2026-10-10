package report

import (
	"sort"
	"time"

	"tock/internal/classify"
	"tock/internal/span"
	"tock/internal/stats"
)

// Unclassified is the category row of time without a category.
const Unclassified = "Unclassified"

// Kinds is the display order of category kinds. "" (unlabelled) is drawn
// after neutral and before distract.
var Kinds = []string{classify.KindDeep, classify.KindShallow, classify.KindNeutral, "", classify.KindDistract}

// DayInput is one local calendar day of labelled spans.
type DayInput struct {
	Date    time.Time // local midnight; its Location is the display zone
	NowMS   int64
	Views   []classify.SpanView // clipped to the day, in start order; the open span ends at NowMS
	Labels  []classify.Label    // Labels[i] labels Views[i]
	Ignored map[string]bool     // inbox keys hidden by tock ignore
	Target  stats.Target        // the day's focus target
	Weights stats.Weights       // score weights
}

// Day is everything the today views show.
type Day struct {
	Date      time.Time
	TrackedMS int64
	IdleMS    int64
	ByKind    map[string]int64 // kind ("" = unlabelled) -> ms; idle excluded
	// Categories: by time desc, then name; Unclassified last.
	Categories []CatRow
	Hours      Heatmap
	Timeline   []Row // chronological
	Stats      stats.Day
	Score      stats.Score
	Goals      []GoalRow           // filled by the caller from the week to date; nil without goals
	Away       []classify.Interval // pending absences, filled by the caller
	Commits    []NameN             // commits per project, filled by the caller
}

// CatRow is one category's time.
type CatRow struct {
	Name, Kind string
	MS         int64
	Projects   []NameMS        // by time desc, then name
	Items      []classify.Item // the category's keys by time; Unclassified omits ignored keys
}

// NameMS is a named total.
type NameMS struct {
	Name string
	MS   int64
}

// Heatmap is time per kind per wall-clock hour.
type Heatmap struct {
	First, Last int                  // hour range to draw, inclusive, within 0..23
	MS          map[string][24]int64 // kind ("" = unlabelled) -> ms per hour; idle excluded
}

// Row is a run of consecutive spans with the same key and label (or a run of
// idle spans).
type Row struct {
	IDs        []int64
	Start, End int64
	Key        string
	App        string
	Text       string // Safe(title, else URL) of the run's last span; "" when private
	Label      classify.Label
	Idle       bool
	Open       bool // the run's last span is the open one (it ends at NowMS)
}

// MinHours is the narrowest heatmap range.
const MinHours = 8

// BuildDay computes the day report.
func BuildDay(in DayInput) Day {
	loc := in.Date.Location()
	d := Day{Date: in.Date, ByKind: map[string]int64{}, Hours: Heatmap{MS: map[string][24]int64{}}}
	cats := map[string]*CatRow{}
	for i, v := range in.Views {
		ms := v.DurMS()
		if v.BundleID == span.IdleBundleID {
			d.IdleMS += ms
			continue
		}
		l := in.Labels[i]
		d.TrackedMS += ms
		d.ByKind[l.Kind] += ms
		hours := d.Hours.MS[l.Kind]
		eachHour(loc, v.Start, v.End, func(h time.Time, part int64) { hours[h.Hour()] += part })
		d.Hours.MS[l.Kind] = hours

		name := l.Category
		if name == "" {
			name = Unclassified
		}
		c := cats[name]
		if c == nil {
			c = &CatRow{Name: name, Kind: l.Kind}
			cats[name] = c
		}
		c.MS += ms
		if l.Project != "" {
			c.Projects = addMS(c.Projects, l.Project, ms)
		}
	}
	for _, c := range cats {
		sortNameMS(c.Projects)
		name := c.Name
		keep := func(i int) bool { return in.Labels[i].Category == name }
		if name == Unclassified {
			keep = func(i int) bool {
				return in.Labels[i].Category == "" && in.Views[i].BundleID != span.IdleBundleID &&
					!in.Ignored[classify.Key(in.Views[i])]
			}
		}
		c.Items = classify.Group(in.Views, keep)
		d.Categories = append(d.Categories, *c)
	}
	sortCats(d.Categories)
	d.Hours.First, d.Hours.Last = hourRange(d.Hours.MS, in.Date, in.NowMS)
	d.Timeline = timeline(in.Views, in.Labels, in.NowMS)
	d.Stats = stats.Compute(in.Views, in.Labels)
	d.Score = stats.ScoreDay(d.Stats, in.Target, in.Weights)
	return d
}

// eachHour splits [startMS, endMS) at local hour boundaries and calls fn with
// each piece's hour start and length. The hour start is found by dropping the
// local minutes and seconds, so DST changes need no special case: a repeated
// hour reports the same wall-clock hour twice.
func eachHour(loc *time.Location, startMS, endMS int64, fn func(hour time.Time, ms int64)) {
	for t := startMS; t < endMS; {
		lt := time.UnixMilli(t).In(loc)
		hStart := t - int64(lt.Minute())*60_000 - int64(lt.Second())*1000 - int64(lt.Nanosecond())/1_000_000
		part := min(hStart+3_600_000, endMS) - t
		fn(time.UnixMilli(hStart).In(loc), part)
		t += part
	}
}

func addMS(list []NameMS, name string, ms int64) []NameMS {
	for i := range list {
		if list[i].Name == name {
			list[i].MS += ms
			return list
		}
	}
	return append(list, NameMS{name, ms})
}

func sortNameMS(list []NameMS) {
	sort.Slice(list, func(a, b int) bool {
		if list[a].MS != list[b].MS {
			return list[a].MS > list[b].MS
		}
		return list[a].Name < list[b].Name
	})
}

func sortCats(c []CatRow) {
	sort.Slice(c, func(a, b int) bool {
		if (c[a].Name == Unclassified) != (c[b].Name == Unclassified) {
			return c[b].Name == Unclassified
		}
		if c[a].MS != c[b].MS {
			return c[a].MS > c[b].MS
		}
		return c[a].Name < c[b].Name
	})
}

// hourRange returns the hours with activity, extended to the current hour on
// today and widened to MinHours; 08..17 for an empty past day.
func hourRange(ms map[string][24]int64, date time.Time, nowMS int64) (int, int) {
	first, last := 24, -1
	for _, hours := range ms {
		for h, v := range hours {
			if v > 0 {
				first, last = min(first, h), max(last, h)
			}
		}
	}
	now := time.UnixMilli(nowMS).In(date.Location())
	if y, m, dd := now.Date(); y == date.Year() && m == date.Month() && dd == date.Day() {
		first, last = min(first, now.Hour()), max(last, now.Hour())
	}
	if last < 0 {
		return 8, 17
	}
	for last-first+1 < MinHours {
		if last < 23 {
			last++
		} else {
			first--
		}
	}
	return first, last
}

func timeline(views []classify.SpanView, labels []classify.Label, nowMS int64) []Row {
	var rows []Row
	for i, v := range views {
		idle := v.BundleID == span.IdleBundleID
		key := ""
		if !idle {
			key = classify.Key(v)
		}
		l := labels[i]
		if n := len(rows); n > 0 {
			r := &rows[n-1]
			if r.Idle == idle && r.Key == key && r.Label.Category == l.Category && r.Label.Project == l.Project {
				r.IDs = append(r.IDs, v.ID)
				r.End = v.End
				r.Text = rowText(v)
				r.Open = v.End == nowMS
				continue
			}
		}
		rows = append(rows, Row{
			IDs: []int64{v.ID}, Start: v.Start, End: v.End, Key: key, App: v.App,
			Text: rowText(v), Label: l, Idle: idle, Open: v.End == nowMS,
		})
	}
	return rows
}

func rowText(v classify.SpanView) string {
	switch {
	case v.Private:
		return ""
	case v.Title != nil && *v.Title != "":
		return Safe(*v.Title)
	case v.URL != nil:
		return Safe(*v.URL)
	}
	return ""
}
