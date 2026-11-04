package classify

import (
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
