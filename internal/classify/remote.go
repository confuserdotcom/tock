package classify

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

// MarkRemote fills the idle pieces and gaps inside [from, to) with the
// other machines' views, so local activity always wins. A filled view keeps
// the remote bundle, title and URL; its app is "<machine> · <app>". Where
// remote views overlap, the one that starts first wins. The result is in
// start order.
func MarkRemote(views []SpanView, marks []RemoteMark, from, to int64) []SpanView {
	if len(marks) == 0 {
		return views
	}
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
