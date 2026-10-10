package classify

import "tock/internal/span"

// Interval is [From, To) in unix ms.
type Interval struct{ From, To int64 }

// MarkCalls splits views at call boundaries. Pieces inside a call get
// InCall, and an idle piece inside a call becomes a span.CallBundleID view:
// a call you only listen to is not idle. Every piece keeps its span's ID.
// calls must be sorted by From and must not overlap.
func MarkCalls(views []SpanView, calls []Interval) []SpanView {
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
