package classify

import (
	"sort"

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

// MarkAway rewrites each mark's idle pieces and gaps inside [from, to) as
// span.AwayBundleID views with ID -mark.ID, so the caller can label them
// through the overrides. Idle views are split at mark edges; other views are
// untouched. The result is in start order.
func MarkAway(views []SpanView, marks []AwayMark, from, to int64) []SpanView {
	iv := make([]Interval, len(marks))
	for i, m := range marks {
		iv[i] = m.Interval
	}
	return fill(views, iv, from, to, func(i int, start, end int64) SpanView {
		return awayView(marks[i].ID, start, end)
	})
}

// fill replaces the idle pieces inside each mark with views made by mk, and
// fills every part of each mark inside [from, to) that no other view covers.
// Idle views are split at mark edges; other views are untouched. The result
// is in start order.
func fill(views []SpanView, marks []Interval, from, to int64, mk func(i int, start, end int64) SpanView) []SpanView {
	if len(marks) == 0 {
		return views
	}
	out := make([]SpanView, 0, len(views)+2*len(marks))
	for _, v := range views {
		if v.BundleID != span.IdleBundleID {
			out = append(out, v)
			continue
		}
		at := v.Start
		for _, m := range marks {
			if m.To <= at || m.From >= v.End {
				continue
			}
			if m.From > at {
				p := v
				p.Start, p.End = at, m.From
				out = append(out, p)
			}
			at = min(m.To, v.End) // the piece inside the mark is filled below
		}
		if at < v.End {
			p := v
			p.Start = at
			out = append(out, p)
		}
	}
	covered := append([]SpanView{}, out...)
	sort.SliceStable(covered, func(i, j int) bool { return covered[i].Start < covered[j].Start })
	for i, m := range marks {
		lo, hi := max(m.From, from), min(m.To, to)
		at := lo
		for _, v := range covered {
			if v.End <= at || v.Start >= hi {
				continue
			}
			if v.Start > at {
				out = append(out, mk(i, at, v.Start))
			}
			at = max(at, v.End)
		}
		if at < hi {
			out = append(out, mk(i, at, hi))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
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
