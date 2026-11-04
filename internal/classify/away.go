package classify

import (
	"tock/internal/span"
)

// Absences returns the stretches with no activity between two non-idle
// views: idle views and gaps with no view, merged. Only absences of at least
// minMS and at most maxMS are returned, in order. views must be in start
// order; call and away views count as activity.
func Absences(views []SpanView, minMS, maxMS int64) []Interval {
	var out []Interval
	var lastEnd int64
	have := false
	for _, v := range views {
		if v.BundleID == span.IdleBundleID {
			continue
		}
		if have {
			if d := v.Start - lastEnd; d >= minMS && d <= maxMS {
				out = append(out, Interval{From: lastEnd, To: v.Start})
			}
			lastEnd = max(lastEnd, v.End)
		} else {
			lastEnd, have = v.End, true
		}
	}
	return out
}

// AwayMark is a labelled absence: the away row's id and its interval.
type AwayMark struct {
	ID int64
	Interval
}

func awayView(id, start, end int64) SpanView {
	return SpanView{ID: -id, Start: start, End: end, BundleID: span.AwayBundleID, App: span.AwayApp}
}

// IsAway reports a labelled away view.
func IsAway(v SpanView) bool { return v.BundleID == span.AwayBundleID }

// Before is the label of what an absence starting at from follows: the
// last view, not idle, that ends at from. Zero when there is none or it has
// no label.
func Before(views []SpanView, labels []Label, from int64) Want {
	for i := len(views) - 1; i >= 0; i-- {
		if v := views[i]; v.BundleID != span.IdleBundleID && v.End == from {
			return Want{Category: labels[i].Category, Project: labels[i].Project}
		}
	}
	return Want{}
}
