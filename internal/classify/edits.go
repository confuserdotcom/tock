package classify

import (
	"sort"

	"tock/internal/span"
)

// EditIDBase keeps edit view ids apart from away, meeting and remote view
// ids: a view an edit made or relabelled has ID -(EditIDBase + edit id).
const EditIDBase = int64(1) << 42

// Edit kinds.
const (
	EditAdd   = "add"   // an entry over the range replaces what is there
	EditCut   = "cut"   // the range is removed
	EditLabel = "label" // the range takes the edit's label; idle time and gaps in it become entries
)

// EditMark is one timeline edit: its row id, kind, interval and, for an
// added entry, its note.
type EditMark struct {
	ID   int64
	Kind string
	Note string
	Interval
}

// EditRowID returns the edit row id of an edit view id, and false when id
// is not one.
func EditRowID(id int64) (int64, bool) {
	if id > -EditIDBase {
		return 0, false
	}
	return -id - EditIDBase, true
}

// IsEntry reports a view an added entry (or a label edit over idle time)
// made.
func IsEntry(v SpanView) bool { return v.BundleID == span.EntryBundleID }

// ApplyEdits applies edits in order (later ones win where they overlap) to
// views, clipped to [from, to). Views are split at edit edges; a piece
// keeps its span's ID unless an edit relabels it. The result is in start
// order. The caller labels the edit ids through the overrides.
func ApplyEdits(views []SpanView, edits []EditMark, from, to int64) []SpanView {
	for _, e := range edits {
		lo, hi := max(e.From, from), min(e.To, to)
		if hi <= lo {
			continue
		}
		id := -(EditIDBase + e.ID)
		out := make([]SpanView, 0, len(views)+3)
		for _, v := range views {
			if v.End <= lo || v.Start >= hi {
				out = append(out, v)
				continue
			}
			if v.Start < lo {
				p := v
				p.End = lo
				out = append(out, p)
			}
			if v.End > hi {
				p := v
				p.Start = hi
				out = append(out, p)
			}
			if e.Kind != EditLabel || v.BundleID == span.IdleBundleID {
				continue // cut, replaced by an entry, or idle: filled below
			}
			p := v
			p.Start, p.End, p.ID = max(v.Start, lo), min(v.End, hi), id
			out = append(out, p)
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
		if e.Kind != EditCut {
			out = fillGaps(out, lo, hi, func(start, end int64) SpanView {
				v := SpanView{ID: id, Start: start, End: end, BundleID: span.EntryBundleID, App: span.EntryApp}
				if e.Note != "" {
					n := e.Note
					v.Title = &n
				}
				return v
			})
		}
		views = out
	}
	return views
}

// fillGaps adds a view made by mk for every part of [lo, hi) that no view
// covers, keeping start order.
func fillGaps(views []SpanView, lo, hi int64, mk func(start, end int64) SpanView) []SpanView {
	at := lo
	var add []SpanView
	for _, v := range views {
		if v.End <= at || v.Start >= hi {
			continue
		}
		if v.Start > at {
			add = append(add, mk(at, v.Start))
		}
		at = max(at, v.End)
	}
	if at < hi {
		add = append(add, mk(at, hi))
	}
	if len(add) == 0 {
		return views
	}
	views = append(views, add...)
	sort.SliceStable(views, func(i, j int) bool { return views[i].Start < views[j].Start })
	return views
}
