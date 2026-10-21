package classify

import "tock/internal/span"

// AFK says when time in front of an app stops counting.
type AFK struct {
	// Window is the AFK window of a view: how long it may go without input
	// (in practice, its category's afk).
	Window func(SpanView) int64
	// StaleMS > 0 means this machine reports no input: a view longer than
	// StaleMS keeps its window and the rest is idle.
	StaleMS int64
	// StaleUntil, when set, limits the stale rule to views that start
	// before it: the machine has reported input since.
	StaleUntil int64
	// TrustMS > 0 is the longest unchanged view this machine's input
	// reporting is trusted with (hypridle can miss idle): a longer view
	// keeps its window and the rest is idle, input or not.
	TrustMS int64
}

// MarkAFK applies the AFK windows to this machine's views, in start order.
// An idle view right after a view p, no longer than p's window, was a pause
// in front of p: p is extended over it. Longer idle stays idle, all of it,
// back to the last input. With StaleMS set, a view longer than StaleMS
// keeps its window and the rest becomes an idle piece with its ID; so does
// a view longer than TrustMS, whatever StaleUntil says. Remote
// and synthetic views are untouched. The input is not modified.
func MarkAFK(views []SpanView, a AFK) []SpanView {
	out := make([]SpanView, 0, len(views)+1)
	for _, v := range views {
		local := v.ID > 0 && v.BundleID != span.IdleBundleID
		if v.BundleID == span.IdleBundleID && len(out) > 0 {
			p := &out[len(out)-1]
			if p.ID > 0 && p.BundleID != span.IdleBundleID && p.End == v.Start && v.DurMS() <= a.Window(*p) {
				p.End = v.End
				continue
			}
		}
		stale := a.StaleMS > 0 && (a.StaleUntil == 0 || v.Start < a.StaleUntil) && v.DurMS() > a.StaleMS
		distrust := a.TrustMS > 0 && v.DurMS() > a.TrustMS
		if cut := v.Start + a.Window(v); local && (stale || distrust) && cut < v.End {
			keep, rest := v, v
			keep.End = cut
			rest.Start, rest.BundleID, rest.App = cut, span.IdleBundleID, span.IdleApp
			rest.Title, rest.URL, rest.Private, rest.Dir, rest.InCall, rest.Kept = nil, nil, false, nil, false, nil
			out = append(out, keep, rest)
			continue
		}
		out = append(out, v)
	}
	return out
}
