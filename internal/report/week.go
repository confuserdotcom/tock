package report

import (
	"time"

	"tock/internal/classify"
	"tock/internal/span"
)

// TopProjects is how many projects a week lists.
const TopProjects = 5

// WeekInput is one Monday-start week of labelled spans.
type WeekInput struct {
	Monday time.Time // local midnight of the week's Monday
	NowMS  int64
	Views  []classify.SpanView // clipped to the week; the open span ends at NowMS
	Labels []classify.Label
}

// Week is what the week view shows.
type Week struct {
	Monday     time.Time
	Days       [7]DayTotal
	TrackedMS  int64
	ByKind     map[string]int64
	Categories []CatRow // Items unused
	Projects   []NameMS // top TopProjects
}

// DayTotal is one day of a week.
type DayTotal struct {
	Date      time.Time
	TrackedMS int64
	ByKind    map[string]int64
	Future    bool // after the day of NowMS
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
	var projects []NameMS
	for i, v := range in.Views {
		if v.BundleID == span.IdleBundleID {
			continue
		}
		l := in.Labels[i]
		ms := v.DurMS()
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
	return w
}

// dayIndex counts calendar days from monday to t's local date.
func dayIndex(monday, t time.Time) int {
	a := time.Date(monday.Year(), monday.Month(), monday.Day(), 12, 0, 0, 0, time.UTC)
	b := time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}
