package focus

import "tock/internal/classify"

// LongestRunMS is the longest activity run in views, ordered by start: the
// run stepBreak measures, over a whole day. An idle view (or time labelled
// away) of at least IdleResetMS, or a gap of at least that length after the
// latest end so far, ends a run; shorter ones do not. A run lasts from its
// first active view's start to its last active view's end, so idle at either
// end does not count. No active view gives 0.
func LongestRunMS(views []classify.SpanView) int64 {
	var best, from, last, lastEnd int64
	open := false
	closeRun := func() {
		if open && last > from {
			best = max(best, last-from)
		}
		open = false
	}
	for _, v := range views {
		switch {
		case isIdle(v):
			if v.DurMS() >= IdleResetMS {
				closeRun()
			}
		default:
			if open && v.Start-lastEnd >= IdleResetMS {
				closeRun()
			}
			if !open {
				from, last, open = v.Start, v.End, true
			}
			last = max(last, v.End)
		}
		lastEnd = max(lastEnd, v.End)
	}
	closeRun()
	return best
}
