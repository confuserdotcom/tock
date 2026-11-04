package classify

import (
	"sort"

	"tock/internal/span"
)

// The six passes that built the timeline before Resolve, in order:
// passCalls, passRemote, passAway, passPlan, passEvents, then ApplyEdits.
// They stay here as the oracle TestResolveMatchesPasses compares Resolve
// with.

// passAway rewrites each mark's idle pieces and gaps inside [from, to) as
// span.AwayBundleID views with ID -mark.ID, so the caller can label them
// through the overrides. Idle views are split at mark edges; other views are
// untouched. The result is in start order.
func passAway(views []SpanView, marks []AwayMark, from, to int64) []SpanView {
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

// passRemote fills the idle pieces and gaps inside [from, to) with the
// other machines' views. You are at one machine at a time: the one that
// switched last. A remote view that starts more than SwitchGraceMS into a
// local view (a recorded span, not a call) takes the time they share; a
// local view that starts later keeps it. A filled view keeps the remote
// bundle, title and URL; its app is "<machine> · <app>". Where remote views
// overlap, the one that starts first wins. The result is in start order.
func passRemote(views []SpanView, marks []RemoteMark, from, to int64) []SpanView {
	if len(marks) == 0 {
		return views
	}
	views = yieldToRemote(views, marks)
	ms := make([]RemoteMark, 0, len(marks))
	var end int64
	for _, m := range marks { // marks come in start order
		if len(ms) > 0 {
			m.View.Start = max(m.View.Start, end)
		}
		if m.View.End <= m.View.Start {
			continue
		}
		ms = append(ms, m)
		end = max(end, m.View.End)
	}
	iv := make([]Interval, len(ms))
	for i, m := range ms {
		iv[i] = Interval{From: m.View.Start, To: m.View.End}
	}
	return fill(views, iv, from, to, func(i int, start, end int64) SpanView {
		v := ms[i].View
		v.ID, v.Start, v.End = -(RemoteIDBase + ms[i].ID), start, end
		v.App = ms[i].Machine + " · " + v.App
		v.Dir, v.InCall = nil, false
		return v
	})
}

// yieldToRemote turns into idle the parts of local views that a remote view
// started later covers (see passRemote), so fill gives them to it.
func yieldToRemote(views []SpanView, marks []RemoteMark) []SpanView {
	out := make([]SpanView, 0, len(views))
	sorted := sort.SliceIsSorted(marks, func(i, j int) bool { return marks[i].View.Start < marks[j].View.Start })
	ends := runningEnds(len(marks), func(i int) int64 { return marks[i].View.End })
	for _, v := range views {
		if v.ID <= 0 || v.InCall || v.BundleID == span.IdleBundleID {
			out = append(out, v)
			continue
		}
		at := v.Start
		first := 0
		if sorted {
			first = firstPast(ends, v.Start)
		}
		for _, m := range marks[first:] { // in start order
			if sorted && m.View.Start >= v.End {
				break
			}
			lo, hi := max(m.View.Start, at), min(m.View.End, v.End)
			if m.View.Start <= v.Start+SwitchGraceMS || hi <= lo {
				continue
			}
			if lo > at {
				p := v
				p.Start, p.End = at, lo
				out = append(out, p)
			}
			p := v
			p.Start, p.End, p.BundleID, p.App = lo, hi, span.IdleBundleID, span.IdleApp
			p.Title, p.URL, p.Private, p.Dir, p.Kept = nil, nil, false, nil, nil
			out = append(out, p)
			at = hi
		}
		if at < v.End {
			p := v
			p.Start = at
			out = append(out, p)
		}
	}
	return out
}

// passEvents rewrites the idle pieces and gaps inside each event, within
// [from, to), as span.MeetingBundleID views titled with the event. Activity
// keeps its own views. Where events overlap, the one that starts first wins.
// The result is in start order.
func passEvents(views []SpanView, events []EventMark, from, to int64) []SpanView {
	evs := append([]EventMark{}, events...)
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].From < evs[j].From })
	marks := make([]EventMark, 0, len(evs))
	var end int64
	for _, e := range evs {
		if len(marks) > 0 {
			e.From = max(e.From, end)
		}
		if e.To <= e.From {
			continue
		}
		marks = append(marks, e)
		end = max(end, e.To)
	}
	iv := make([]Interval, len(marks))
	for i, m := range marks {
		iv[i] = m.Interval
	}
	// An event is more specific than a call: call time inside it becomes
	// the meeting. Call views go in as idle (InCall tells them apart) and
	// what no event claims is call time again.
	in := make([]SpanView, len(views))
	for i, v := range views {
		if v.BundleID == span.CallBundleID {
			v.BundleID, v.App = span.IdleBundleID, span.IdleApp
		}
		in[i] = v
	}
	out := fill(in, iv, from, to, func(i int, start, end int64) SpanView {
		title := marks[i].Title
		return SpanView{ID: -(MeetingIDBase + marks[i].ID), Start: start, End: end,
			BundleID: span.MeetingBundleID, App: span.MeetingApp, Title: &title}
	})
	for i := range out {
		if out[i].BundleID == span.IdleBundleID && out[i].InCall {
			out[i].BundleID, out[i].App = span.CallBundleID, span.CallApp
		}
	}
	return out
}

// passPlan splits views at slot edges and sets Planned on the pieces
// inside a slot. Every piece keeps its span's ID. slots must be sorted by
// From and must not overlap.
func passPlan(views []SpanView, slots []PlanSlot) []SpanView {
	if len(slots) == 0 {
		return views
	}
	out := make([]SpanView, 0, len(views)+2*len(slots))
	for _, v := range views {
		at := v.Start
		for _, s := range slots {
			if s.To <= at || s.From >= v.End {
				continue
			}
			if s.From > at {
				p := v
				p.Start, p.End = at, s.From
				out = append(out, p)
				at = s.From
			}
			p := v
			p.Start, p.End, p.Planned = at, min(s.To, v.End), s.Project
			out = append(out, p)
			at = p.End
		}
		if at < v.End {
			p := v
			p.Start = at
			out = append(out, p)
		}
	}
	return out
}

// passCalls splits views at call boundaries. Pieces inside a call get
// InCall, and an idle piece inside a call becomes a span.CallBundleID view:
// a call you only listen to is not idle. Every piece keeps its span's ID.
// calls must be sorted by From and must not overlap.
func passCalls(views []SpanView, calls []Interval) []SpanView {
	if len(calls) == 0 {
		return views
	}
	out := make([]SpanView, 0, len(views)+2*len(calls))
	for _, v := range views {
		at := v.Start
		for _, c := range calls {
			if c.To <= at || c.From >= v.End {
				continue
			}
			if c.From > at {
				p := v
				p.Start, p.End = at, c.From
				out = append(out, p)
				at = c.From
			}
			p := v
			p.Start, p.End, p.InCall = at, min(c.To, v.End), true
			if p.BundleID == span.IdleBundleID {
				p.BundleID, p.App, p.Title, p.URL = span.CallBundleID, span.CallApp, nil, nil
			}
			out = append(out, p)
			at = p.End
		}
		if at < v.End {
			p := v
			p.Start = at
			out = append(out, p)
		}
	}
	return out
}
