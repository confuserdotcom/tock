package classify

import ()

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
