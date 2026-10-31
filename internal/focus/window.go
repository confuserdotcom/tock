package focus

import (
	"slices"
	"strings"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/stats"
)

const (
	// topN is how many categories and apps a Breakdown lists.
	topN = 3
	// minShareMS is the time a category or app needs to be listed: report.Dur
	// would print seconds below a minute.
	minShareMS = 60_000
)

// Share is the time spent on one named thing (a category or an app).
type Share struct {
	Name string
	MS   int64
}

// Breakdown is what a time window was spent on, for the session card, the
// done notice and refile.
type Breakdown struct {
	Categories, Apps []Share // top 3 by MS descending, ties by Name ascending; under a minute dropped
	Project          string  // the project with the most deep time; ties by name ascending; "" = none
	ProjectMS        int64   // non-idle time in the window labelled Project
	ActiveMS         int64   // non-idle (not idle, not away) time in the window
}

// Window breaks the half-open window [from, to) down. Idle and away views
// count toward nothing. cfg is accepted for parity with WindowStats: meeting
// views count like any other non-idle view here.
func Window(views []classify.SpanView, labels []classify.Label, from, to int64, cfg config.Focus) Breakdown {
	_ = cfg
	vs, ls := stats.Clip(views, labels, from, to)
	cats, apps := map[string]int64{}, map[string]int64{}
	projs, deep := map[string]int64{}, map[string]int64{}
	var b Breakdown
	for i, v := range vs {
		if isIdle(v) {
			continue
		}
		d, l := v.DurMS(), ls[i]
		b.ActiveMS += d
		if l.Category != "" {
			cats[l.Category] += d
		}
		if v.App != "" {
			apps[v.App] += d
		}
		if l.Project != "" {
			projs[l.Project] += d
			if l.Kind == classify.KindDeep {
				deep[l.Project] += d
			}
		}
	}
	b.Categories, b.Apps = top(cats), top(apps)
	for name, ms := range deep {
		if ms > 0 && (ms > deep[b.Project] || (ms == deep[b.Project] && name < b.Project)) {
			b.Project = name
		}
	}
	b.ProjectMS = projs[b.Project]
	return b
}

// top is the first topN entries of m by time descending then name
// ascending, leaving out those under a minute.
func top(m map[string]int64) []Share {
	var out []Share
	for name, ms := range m {
		if ms >= minShareMS {
			out = append(out, Share{Name: name, MS: ms})
		}
	}
	slices.SortFunc(out, func(a, b Share) int {
		if a.MS != b.MS {
			if a.MS > b.MS {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	if len(out) > topN {
		out = out[:topN]
	}
	return out
}

// Refile reports whether a refile to the top project is worth offering: a
// top project exists and some non-idle time is not labelled with it.
func (b Breakdown) Refile() bool { return b.Project != "" && b.ProjectMS < b.ActiveMS }

// ActiveRuns are the maximal runs of non-idle views inside [from, to), in
// time order. Idle, away and untracked gaps are never inside a run, so a
// refile that labels exactly these runs cannot turn them into project time.
func ActiveRuns(views []classify.SpanView, from, to int64) []classify.Interval {
	vs, _ := stats.Clip(views, make([]classify.Label, len(views)), from, to)
	var runs []classify.Interval
	for _, v := range vs {
		if isIdle(v) {
			continue
		}
		if n := len(runs); n > 0 && v.Start <= runs[n-1].To {
			runs[n-1].To = max(runs[n-1].To, v.End)
			continue
		}
		runs = append(runs, classify.Interval{From: v.Start, To: v.End})
	}
	return runs
}
