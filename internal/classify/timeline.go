package classify

import (
	"sort"

	"tock/internal/span"
)

// Sources is everything that claims time on the timeline besides this
// machine's spans. No slice needs to be sorted.
type Sources struct {
	Calls  []Interval   // a camera or microphone in use (long enough to count)
	Remote []RemoteMark // the other machines' views
	Away   []AwayMark   // labelled absences
	Events []EventMark  // calendar events; nil when no category takes them
	Plan   []PlanSlot   // the plan's timed tasks and their modules
	Edits  []EditMark   // timeline edits; the later id wins where they overlap
	NowMS  int64        // remote views, away time, events and edits stop here; 0 = no limit
}

// Resolve lays the sources over this machine's views, clipped to
// [from, to), in one pass, and returns the timeline in start order. Pass
// the views whole: the switch rule compares when they began, not where a
// day cuts them.
//
// Each instant has one owner, decided by this table:
//
//	in a call (you are at this machine: other machines claim nothing)
//	  activity          keeps its view, InCall
//	  idle time         a calendar event's meeting, else a call view
//	outside a call
//	  activity          keeps its view, unless another machine's view that
//	                    started more than SwitchGraceMS later covers it:
//	                    you are at the machine you switched to last
//	  idle time, gaps   another machine's view, else a labelled absence,
//	                    else a calendar event's meeting, else stays idle
//	                    (a gap stays empty)
//	edits               win over all of the above, the later one first
//
// A call only covers this machine's spans: trackd records idle time during
// a call, so a gap means nothing was recording. Planned is context, never
// ownership: every piece a plan slot covers gets it, except meetings, which
// the event explains. Where claims of one kind overlap, the one that
// starts first wins (then the lower id). Remote views, absences, events and
// edits stop at NowMS.
//
// Invariants (see TestResolveProperties): no two pieces overlap, even
// where views do, every instant of a view or claim inside [from, min(to,
// NowMS)) that no edit cuts is covered exactly once, and the result does
// not depend on the order of any source slice. Adjacent pieces of one
// owner with the same fields are merged.
func Resolve(views []SpanView, src Sources, from, to int64) []SpanView {
	hi := to
	if src.NowMS != 0 {
		hi = min(to, src.NowMS)
	}
	calls := claimsOf(len(src.Calls), func(i int) (Interval, int64) { return src.Calls[i], int64(i) }, from, to)
	plan := claimsOf(len(src.Plan), func(i int) (Interval, int64) { return src.Plan[i].Interval, int64(i) }, from, to)
	remote := claimsOf(len(src.Remote), func(i int) (Interval, int64) {
		v := src.Remote[i].View
		return Interval{v.Start, v.End}, src.Remote[i].ID
	}, from, hi)
	away := claimsOf(len(src.Away), func(i int) (Interval, int64) { return src.Away[i].Interval, src.Away[i].ID }, from, hi)
	events := claimsOf(len(src.Events), func(i int) (Interval, int64) { return src.Events[i].Interval, src.Events[i].ID }, from, hi)
	// One remote view value and one meeting title per source, so the
	// pieces of one owner share their pointers and merge.
	remoteViews := make([]SpanView, len(src.Remote))
	for i, m := range src.Remote {
		v := m.View
		v.ID, v.App, v.Dir, v.InCall = -(RemoteIDBase + m.ID), m.Machine+" · "+v.App, nil, false
		remoteViews[i] = v
	}
	titles := make([]*string, len(src.Events))
	for i := range src.Events {
		t := src.Events[i].Title
		titles[i] = &t
	}

	cuts := []int64{from, to, hi}
	for _, c := range []claims{calls, plan, remote, away, events} {
		cuts = append(cuts, c.from...)
		cuts = append(cuts, c.to...)
	}
	vs, began := clipSorted(views, from, to)
	for _, v := range vs {
		cuts = append(cuts, v.Start, v.End)
	}
	cuts = uniqueSorted(cuts)

	out := make([]SpanView, 0, len(vs)+len(remote.from)+len(away.from)+len(events.from))
	emit := func(p SpanView, s, e int64) {
		p.Start, p.End = s, e
		if n := len(out); n > 0 && samePiece(out[n-1], p) {
			out[n-1].End = e
			return
		}
		out = append(out, p)
	}
	// piece decides the owner of [s, e), where one claim of each kind
	// holds throughout; v is the view of this machine there, or nil.
	piece := func(v *SpanView, began, s, e int64) {
		inCall := v != nil && calls.at(s) >= 0
		planned := ""
		if i := plan.at(s); i >= 0 {
			planned = src.Plan[plan.src[i]].Project
		}
		idle := v == nil || v.BundleID == span.IdleBundleID
		var p SpanView
		switch {
		case inCall && !idle:
			p = *v
			p.InCall = true
		case inCall:
			if i := events.at(s); i >= 0 {
				emit(meetingView(src.Events[events.src[i]].ID, titles[events.src[i]]), s, e)
				return
			}
			p = *v
			p.BundleID, p.App, p.Title, p.URL, p.InCall = span.CallBundleID, span.CallApp, nil, nil, true
		case !idle:
			p = *v
			if i := remote.at(s); i >= 0 && v.ID > 0 && src.Remote[remote.src[i]].View.Start > began+SwitchGraceMS {
				p = remoteViews[remote.src[i]]
			}
		default:
			if i := remote.at(s); i >= 0 {
				p = remoteViews[remote.src[i]]
			} else if i := away.at(s); i >= 0 {
				p = awayView(src.Away[away.src[i]].ID, 0, 0)
			} else if i := events.at(s); i >= 0 {
				emit(meetingView(src.Events[events.src[i]].ID, titles[events.src[i]]), s, e)
				return
			} else if v != nil {
				p = *v
			} else {
				return // a gap nothing claims
			}
		}
		p.Planned = planned
		emit(p, s, e)
	}

	// One sweep over the cuts: each piece has at most one view of this
	// machine. Where views overlap (a span left open by a crash), the one
	// that started last wins: you are where you switched last, and an idle
	// span starts at the last input.
	var active []int
	j := 0
	for k := 0; k+1 < len(cuts); k++ {
		s, e := cuts[k], cuts[k+1]
		for j < len(vs) && vs[j].Start <= s {
			active = append(active, j)
			j++
		}
		n := 0
		var v *SpanView
		var vBegan int64
		for _, i := range active {
			if vs[i].End <= s {
				continue
			}
			active[n] = i
			n++
			v, vBegan = &vs[i], began[i] // active is in start order
		}
		active = active[:n]
		piece(v, vBegan, s, e)
	}

	if len(src.Edits) == 0 {
		return out
	}
	edits := append([]EditMark(nil), src.Edits...)
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].ID < edits[j].ID })
	return ApplyEdits(out, edits, from, hi)
}

func meetingView(id int64, title *string) SpanView {
	return SpanView{ID: -(MeetingIDBase + id), BundleID: span.MeetingBundleID, App: span.MeetingApp, Title: title}
}

// claims is one kind of claim, clipped, in start order and without
// overlaps: src[i] is the index in the source slice of from[i]..to[i].
type claims struct {
	from, to []int64
	src      []int
}

// claimsOf sorts n claims by start then id, cuts each one's overlap with
// those before it (the first to start wins), and clips them to [lo, hi).
func claimsOf(n int, get func(i int) (Interval, int64), lo, hi int64) claims {
	type item struct {
		iv  Interval
		id  int64
		src int
	}
	items := make([]item, 0, n)
	for i := 0; i < n; i++ {
		iv, id := get(i)
		items = append(items, item{iv, id, i})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].iv.From != items[j].iv.From {
			return items[i].iv.From < items[j].iv.From
		}
		return items[i].id < items[j].id
	})
	var c claims
	end := lo
	for _, it := range items {
		s, e := max(it.iv.From, end, lo), min(it.iv.To, hi)
		if e <= s {
			continue
		}
		c.from, c.to, c.src = append(c.from, s), append(c.to, e), append(c.src, it.src)
		end = e
	}
	return c
}

// at is the index of the claim that holds at t, or -1.
func (c claims) at(t int64) int {
	i := sort.Search(len(c.from), func(i int) bool { return c.from[i] > t }) - 1
	if i >= 0 && c.to[i] > t {
		return i
	}
	return -1
}

// clipSorted is views cut to [from, to), the empty ones dropped, in start
// order, with when each began before it was cut.
func clipSorted(views []SpanView, from, to int64) ([]SpanView, []int64) {
	type item struct {
		v     SpanView
		began int64
	}
	items := make([]item, 0, len(views))
	for _, v := range views {
		b := v.Start
		v.Start, v.End = max(v.Start, from), min(v.End, to)
		if v.End > v.Start {
			items = append(items, item{v, b})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].v.Start < items[j].v.Start })
	out, began := make([]SpanView, len(items)), make([]int64, len(items))
	for i, it := range items {
		out[i], began[i] = it.v, it.began
	}
	return out, began
}

func uniqueSorted(xs []int64) []int64 {
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	out := xs[:0]
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}

// samePiece reports whether b continues a: the same owner and fields,
// starting where a ends.
func samePiece(a, b SpanView) bool {
	return a.End == b.Start && a.ID == b.ID && a.BundleID == b.BundleID && a.App == b.App &&
		a.Title == b.Title && a.URL == b.URL && a.Private == b.Private && a.Dir == b.Dir &&
		a.InCall == b.InCall && a.Kept == b.Kept && a.Planned == b.Planned &&
		len(a.Screen) == len(b.Screen) && (len(a.Screen) == 0 || &a.Screen[0] == &b.Screen[0])
}
