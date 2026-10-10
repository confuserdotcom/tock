package report

import (
	"strings"
	"time"

	"tock/internal/classify"
	"tock/internal/span"
	"tock/internal/stats"
)

// TopProjects is how many projects, and how many apps and sites, a week
// lists.
const TopProjects = 5

// WeekInput is one Monday-start week of labelled spans.
type WeekInput struct {
	Monday  time.Time // local midnight of the week's Monday
	NowMS   int64
	Views   []classify.SpanView // clipped to the week; the open span ends at NowMS
	Labels  []classify.Label
	Targets [7]stats.Target // each day's focus target; future days are not scored
	Weights stats.Weights
	Goals   []classify.Goal
}

// Week is what the week view shows.
type Week struct {
	Monday     time.Time
	Days       [7]DayTotal
	TrackedMS  int64
	ByKind     map[string]int64
	Categories []CatRow // Items unused
	Projects   []NameMS // top TopProjects
	Apps       []NameMS // top TopProjects apps and sites (see AppName)
	Goals      []WeekGoalRow
	// Distractions are the top TopProjects apps and sites in distract-kind
	// categories.
	Distractions []NameMS
	Plan         string  // the week's plan from tock review; filled by the caller
	Commits      []NameN // commits per project, filled by the caller
}

// DayTotal is one day of a week.
type DayTotal struct {
	Date      time.Time
	TrackedMS int64
	ByKind    map[string]int64
	Future    bool // after the day of NowMS
	Stats     stats.Day
	Score     stats.Score
}

// MondayOf returns local midnight of the Monday on or before t.
func MondayOf(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	back := (int(d.Weekday()) + 6) % 7 // Monday = 0
	return d.AddDate(0, 0, -back)
}

// BuildWeek computes the week report.
func BuildWeek(in WeekInput) Week {
	loc := in.Monday.Location()
	w := Week{Monday: in.Monday, ByKind: map[string]int64{}}
	now := time.UnixMilli(in.NowMS).In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	for i := range w.Days {
		date := in.Monday.AddDate(0, 0, i)
		w.Days[i] = DayTotal{Date: date, ByKind: map[string]int64{}, Future: date.After(today)}
	}
	cats := map[string]*CatRow{}
	var projects, apps, distract []NameMS
	for i, v := range in.Views {
		if v.BundleID == span.IdleBundleID {
			continue
		}
		l := in.Labels[i]
		ms := v.DurMS()
		apps = addMS(apps, AppName(v), ms)
		if l.Kind == classify.KindDistract {
			distract = addMS(distract, AppName(v), ms)
		}
		w.TrackedMS += ms
		w.ByKind[l.Kind] += ms
		eachHour(loc, v.Start, v.End, func(h time.Time, part int64) {
			if di := dayIndex(in.Monday, h); di >= 0 && di < 7 {
				w.Days[di].TrackedMS += part
				w.Days[di].ByKind[l.Kind] += part
			}
		})
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
			projects = addMS(projects, l.Project, ms)
		}
	}
	for _, c := range cats {
		sortNameMS(c.Projects)
		w.Categories = append(w.Categories, *c)
	}
	sortCats(w.Categories)
	sortNameMS(projects)
	w.Projects = projects[:min(len(projects), TopProjects)]
	sortNameMS(apps)
	w.Apps = apps[:min(len(apps), TopProjects)]
	sortNameMS(distract)
	w.Distractions = distract[:min(len(distract), TopProjects)]
	if len(in.Goals) > 0 {
		w.Goals = WeekGoals(in.Goals, in.Views, in.Labels, in.Monday, in.NowMS)
	}
	for i, day := range splitDays(in.Monday, in.Views, in.Labels) {
		if w.Days[i].Future {
			continue
		}
		w.Days[i].Stats = stats.Compute(day.views, day.labels)
		w.Days[i].Score = stats.ScoreDay(w.Days[i].Stats, in.Targets[i], in.Weights)
	}
	return w
}

// AppName names a view for the week's apps and sites: the host of its URL
// without a leading "www." when it has one and is not private, else its app.
func AppName(v classify.SpanView) string {
	if v.Private || v.URL == nil || *v.URL == "" {
		return v.App
	}
	host, _, _ := strings.Cut(*v.URL, "/")
	return strings.TrimPrefix(host, "www.")
}

type dayViews struct {
	views  []classify.SpanView
	labels []classify.Label
}

// splitDays clips the week's views to each of its seven local days.
func splitDays(monday time.Time, views []classify.SpanView, labels []classify.Label) [7]dayViews {
	var out [7]dayViews
	for i := range out {
		from, to := monday.AddDate(0, 0, i).UnixMilli(), monday.AddDate(0, 0, i+1).UnixMilli()
		for j, v := range views {
			if v.End <= from || v.Start >= to {
				continue
			}
			v.Start, v.End = max(v.Start, from), min(v.End, to)
			out[i].views = append(out[i].views, v)
			out[i].labels = append(out[i].labels, labels[j])
		}
	}
	return out
}

// dayIndex counts calendar days from monday to t's local date.
func dayIndex(monday, t time.Time) int {
	a := time.Date(monday.Year(), monday.Month(), monday.Day(), 12, 0, 0, 0, time.UTC)
	b := time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}
