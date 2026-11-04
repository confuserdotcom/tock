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
	// Marks in start order let each view skip those that ended before it
	// and stop at the first that starts after it; the result is the same.
	sorted := sort.SliceIsSorted(marks, func(i, j int) bool { return marks[i].From < marks[j].From })
	markEnds := runningEnds(len(marks), func(i int) int64 { return marks[i].To })
	for _, v := range views {
		if v.BundleID != span.IdleBundleID {
			out = append(out, v)
			continue
		}
		at := v.Start
		first := 0
		if sorted {
			first = firstPast(markEnds, v.Start)
		}
		for _, m := range marks[first:] {
			if sorted && m.From >= v.End {
				break
			}
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
	coveredEnds := runningEnds(len(covered), func(i int) int64 { return covered[i].End })
	for i, m := range marks {
		lo, hi := max(m.From, from), min(m.To, to)
		at := lo
		for _, v := range covered[firstPast(coveredEnds, lo):] {
			if v.Start >= hi {
				break // covered is in start order
			}
			if v.End <= at {
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

// runningEnds is, for n items in start order, the latest end among items
// 0..i: it never decreases, so firstPast can search it.
func runningEnds(n int, end func(i int) int64) []int64 {
	out := make([]int64, n)
	var m int64
	for i := range out {
		m = max(m, end(i))
		out[i] = m
	}
	return out
}

// firstPast is the first index whose running end is after t: every item
// before it ended by t.
func firstPast(ends []int64, t int64) int {
	return sort.Search(len(ends), func(i int) bool { return ends[i] > t })
}
