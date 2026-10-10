package classify

import (
	"sort"

	"tock/internal/span"
)

// MeetingIDBase keeps meeting view ids apart from away view ids (-away row
// id): a meeting view has ID -(MeetingIDBase + event id).
const MeetingIDBase = int64(1) << 40

// EventMark is a calendar event: its row id, title and interval.
type EventMark struct {
	ID    int64
	Title string
	Interval
}

// MeetingEventID returns the event id of a meeting view id, and false when
// id is not one.
func MeetingEventID(id int64) (int64, bool) {
	if id > -MeetingIDBase || id <= -RemoteIDBase {
		return 0, false
	}
	return -id - MeetingIDBase, true
}

// IsMeeting reports a view that a calendar event explains.
func IsMeeting(v SpanView) bool { return v.BundleID == span.MeetingBundleID }

// MarkEvents rewrites the idle pieces and gaps inside each event, within
// [from, to), as span.MeetingBundleID views titled with the event. Activity
// keeps its own views. Where events overlap, the one that starts first wins.
// The result is in start order.
func MarkEvents(views []SpanView, events []EventMark, from, to int64) []SpanView {
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
	return fill(views, iv, from, to, func(i int, start, end int64) SpanView {
		title := marks[i].Title
		return SpanView{ID: -(MeetingIDBase + marks[i].ID), Start: start, End: end,
			BundleID: span.MeetingBundleID, App: span.MeetingApp, Title: &title}
	})
}
