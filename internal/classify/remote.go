package classify

import (
	"sort"

	"tock/internal/span"
)

// RemoteIDBase keeps remote view ids apart from away and meeting view ids:
// a remote view has ID -(RemoteIDBase + remote row id).
const RemoteIDBase = int64(1) << 41

// RemoteMark is one view of another machine: its row id, the machine, and
// the view, whose ID is ignored.
type RemoteMark struct {
	ID      int64
	Machine string
	View    SpanView
}

// RemoteRowID returns the remote row id of a remote view id, and false when
// id is not one.
func RemoteRowID(id int64) (int64, bool) {
	if id > -RemoteIDBase || id <= -EditIDBase {
		return 0, false
	}
	return -id - RemoteIDBase, true
}

// IsRemote reports a view of another machine's activity.
func IsRemote(v SpanView) bool {
	_, ok := RemoteRowID(v.ID)
	return ok
}

// SwitchGraceMS is how much later than a local view a remote one must
// start to take its time: clock skew between machines, and a flicker.
const SwitchGraceMS = 30_000

// MarkRemote fills the idle pieces and gaps inside [from, to) with the
// other machines' views. You are at one machine at a time: the one that
// switched last. A remote view that starts more than SwitchGraceMS into a
// local view (a recorded span, not a call) takes the time they share; a
// local view that starts later keeps it. A filled view keeps the remote
// bundle, title and URL; its app is "<machine> · <app>". Where remote views
// overlap, the one that starts first wins. The result is in start order.
func MarkRemote(views []SpanView, marks []RemoteMark, from, to int64) []SpanView {
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
// started later covers (see MarkRemote), so fill gives them to it.
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
